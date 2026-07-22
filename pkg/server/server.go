package server

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"remote-container-manager/pkg/auth"
	"remote-container-manager/pkg/docker"
	"remote-container-manager/pkg/logger"

	"github.com/nats-io/nats.go"
)

// config holds server configuration options
type Config struct {
	NatsURL          string
	ContainerName    string
	Subject          string
	KeysFile         string
	SingleKey        string
	ChallengeTTL     time.Duration
	HeartbeatTimeout time.Duration
	Trace            bool
}

// server manages container control server instance
type Server struct {
	config     Config
	authMgr    *auth.Manager
	dockerMgr  *docker.Manager
	sessionMgr *SessionManager
}

// new creates a new server instance with given configuration
func New(cfg Config) (*Server, error) {
	// default subject format if not specified
	if cfg.Subject == "" {
		cfg.Subject = fmt.Sprintf("container.%s.control", cfg.ContainerName)
	}

	if cfg.ChallengeTTL <= 0 {
		cfg.ChallengeTTL = 30 * time.Second
	}

	if cfg.HeartbeatTimeout <= 0 {
		cfg.HeartbeatTimeout = 10 * time.Second
	}

	// initialize auth manager
	authMgr := auth.NewManager(cfg.ChallengeTTL)

	// load authorized public keys from file
	if cfg.KeysFile != "" {
		if err := authMgr.LoadAuthorizedKeysFile(cfg.KeysFile); err != nil {
			return nil, fmt.Errorf("failed to load authorized keys file: %w", err)
		}
		logger.Info("loaded authorized public keys from file: %s", cfg.KeysFile)
	}

	// load single key if provided
	if cfg.SingleKey != "" {
		authMgr.LoadAuthorizedKeys([]string{cfg.SingleKey})
		logger.Info("added single authorized public key from options")
	}

	// initialize docker manager
	dockerMgr, err := docker.NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize docker manager: %w", err)
	}

	// initialize session manager for heartbeat monitoring
	sessionMgr := NewSessionManager(cfg.HeartbeatTimeout)

	return &Server{
		config:     cfg,
		authMgr:    authMgr,
		dockerMgr:  dockerMgr,
		sessionMgr: sessionMgr,
	}, nil
}

// run starts server listening loop and blocks until OS signal is received
func (s *Server) Run() error {
	defer s.dockerMgr.Close()

	logger.Info("starting container manager server for container: %s", s.config.ContainerName)
	logger.Info("listening on nats subject: %s", s.config.Subject)
	logger.Info("heartbeat timeout set to: %s", s.config.HeartbeatTimeout)
	if s.config.Trace {
		logger.Info("nats message tracing enabled ([TRC] <<<<< / >>>>>)")
	}

	// connect to nats server
	nc, err := nats.Connect(s.config.NatsURL)
	if err != nil {
		return fmt.Errorf("failed to connect to nats at %s: %w", s.config.NatsURL, err)
	}
	defer nc.Close()

	logger.Info("connected to nats server at %s", s.config.NatsURL)

	// subscribe to nats subject
	_, err = nc.Subscribe(s.config.Subject, func(msg *nats.Msg) {
		// handle incoming request in a goroutine
		go handleIncomingMsg(nc, msg, s.authMgr, s.dockerMgr, s.sessionMgr, s.config.ContainerName, s.config.Trace)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to subject %s: %w", s.config.Subject, err)
	}

	logger.Info("server is ready and waiting for requests...")

	// wait for OS signal for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down container manager server...")
	return nil
}
