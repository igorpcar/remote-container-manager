package client

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"remote-container-manager/pkg/auth"
	"remote-container-manager/pkg/logger"
	"remote-container-manager/pkg/protocol"

	"github.com/nats-io/nats.go"
)

// config holds client configuration options
type Config struct {
	NatsURL       string
	ContainerName string
	Action        string
	Subject       string
	PubKeyInput   string
	PrivKeyInput  string
	Timeout       time.Duration
	Trace         bool
	Heartbeat     bool
}

// client manages nats client connection and challenge execution
type Client struct {
	config    Config
	pubKeyHex string
	privKey   ed25519.PrivateKey
}

// new creates a new client instance and loads cryptographic keys
func New(cfg Config) (*Client, error) {
	if cfg.Subject == "" {
		cfg.Subject = fmt.Sprintf("container.%s.control", cfg.ContainerName)
	}

	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}

	// load public and private keys
	pubKeyHex, privKey, err := LoadKeys(cfg.PubKeyInput, cfg.PrivKeyInput)
	if err != nil {
		return nil, fmt.Errorf("failed to load client keys: %w", err)
	}

	return &Client{
		config:    cfg,
		pubKeyHex: pubKeyHex,
		privKey:   privKey,
	}, nil
}

// executeaction executes full challenge-response workflow over nats
func (c *Client) ExecuteAction() error {
	logger.Info("connecting to nats server at %s...", c.config.NatsURL)
	if c.config.Trace {
		logger.Info("nats message tracing enabled ([TRC] <<<<< / >>>>>)")
	}

	nc, err := nats.Connect(c.config.NatsURL)
	if err != nil {
		return fmt.Errorf("failed to connect to nats: %w", err)
	}
	defer nc.Close()

	logger.Info("sending request for container '%s' action '%s' on subject '%s'...", c.config.ContainerName, c.config.Action, c.config.Subject)

	// phase 1: request challenge from server
	resp1, err := c.requestChallenge(nc)
	if err != nil {
		return fmt.Errorf("challenge initiation failed: %w", err)
	}

	// phase 2: verify challenge with signature
	resp2, err := c.verifyChallenge(nc, resp1)
	if err != nil {
		return fmt.Errorf("challenge verification failed: %w", err)
	}

	// print execution result summary
	c.printResult(resp2)

	// if heartbeat session was established, start active heartbeat ping loop
	if resp2.SessionID != "" && c.config.Heartbeat && c.config.Action == protocol.ActionStart {
		c.startHeartbeatLoop(nc, resp2.SessionID, resp2.HeartbeatInterval)
	}

	return nil
}

// requestchallenge sends initial challenge request to server
func (c *Client) requestChallenge(nc *nats.Conn) (*protocol.Response, error) {
	initReq := protocol.Request{
		Type:      protocol.TypeInitChallenge,
		Action:    c.config.Action,
		PublicKey: c.pubKeyHex,
		Heartbeat: c.config.Heartbeat,
	}

	initData, err := json.Marshal(initReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal init request: %w", err)
	}

	if c.config.Trace {
		logger.TraceOut(c.config.Subject, initData)
	}

	msgResp1, err := nc.Request(c.config.Subject, initData, c.config.Timeout)
	if err != nil {
		return nil, fmt.Errorf("nats request failed: %w", err)
	}

	if c.config.Trace {
		logger.TraceIn(msgResp1.Subject, "", msgResp1.Data)
	}

	var resp1 protocol.Response
	if err := json.Unmarshal(msgResp1.Data, &resp1); err != nil {
		return nil, fmt.Errorf("failed to parse response json: %w", err)
	}

	if resp1.Type == protocol.TypeError || resp1.Status == "unauthorized" {
		return nil, fmt.Errorf("server rejected request: status=%s, error=%s", resp1.Status, resp1.Error)
	}

	if resp1.Type != protocol.TypeChallengeResp {
		return nil, fmt.Errorf("unexpected response type received: %s", resp1.Type)
	}

	logger.Info("received challenge response from server (challenge_id: %s)", resp1.ChallengeID)
	return &resp1, nil
}

// verifychallenge signs challenge payload and sends signature to server for execution
func (c *Client) verifyChallenge(nc *nats.Conn, resp1 *protocol.Response) (*protocol.Response, error) {
	// decode raw challenge bytes
	challengeRawBytes, err := base64.StdEncoding.DecodeString(resp1.ChallengeData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode challenge data: %w", err)
	}

	// sign challenge payload with client private key
	signatureB64 := auth.SignChallenge(c.privKey, challengeRawBytes)

	verifyReq := protocol.Request{
		Type:        protocol.TypeVerifyChallenge,
		Action:      c.config.Action,
		PublicKey:   c.pubKeyHex,
		ChallengeID: resp1.ChallengeID,
		Signature:   signatureB64,
		Heartbeat:   c.config.Heartbeat,
	}

	verifyData, err := json.Marshal(verifyReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal verify request: %w", err)
	}

	logger.Info("sending signed challenge response to server...")
	if c.config.Trace {
		logger.TraceOut(c.config.Subject, verifyData)
	}

	msgResp2, err := nc.Request(c.config.Subject, verifyData, c.config.Timeout)
	if err != nil {
		return nil, fmt.Errorf("nats verify request failed: %w", err)
	}

	if c.config.Trace {
		logger.TraceIn(msgResp2.Subject, "", msgResp2.Data)
	}

	var resp2 protocol.Response
	if err := json.Unmarshal(msgResp2.Data, &resp2); err != nil {
		return nil, fmt.Errorf("failed to parse verify response json: %w", err)
	}

	if resp2.Type == protocol.TypeError {
		return nil, fmt.Errorf("action failed on server: status=%s, error=%s", resp2.Status, resp2.Error)
	}

	return &resp2, nil
}

// startheartbeatloop sends periodic ping requests to keep container active until client exits
func (c *Client) startHeartbeatLoop(nc *nats.Conn, sessionID string, intervalSec int) {
	if intervalSec <= 0 {
		intervalSec = 3
	}

	interval := time.Duration(intervalSec) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("heartbeat session established! (session_id: %s)", sessionID)
	logger.Info("sending heartbeat ping every %v... (press Ctrl+C to stop client and trigger container shutdown)", interval)

	for {
		select {
		case <-ticker.C:
			hbReq := protocol.Request{
				Type:      protocol.TypeHeartbeat,
				SessionID: sessionID,
				PublicKey: c.pubKeyHex,
			}
			hbData, _ := json.Marshal(hbReq)

			if c.config.Trace {
				logger.TraceOut(c.config.Subject, hbData)
			}

			hbMsg, err := nc.Request(c.config.Subject, hbData, c.config.Timeout)
			if err != nil {
				logger.Error("failed to send heartbeat ping: %v", err)
				continue
			}

			if c.config.Trace {
				logger.TraceIn(hbMsg.Subject, "", hbMsg.Data)
			}

		case <-sigChan:
			logger.Info("client application shutting down. heartbeat stopped.")
			logger.Info("server will detect heartbeat timeout and automatically stop container '%s'.", c.config.ContainerName)
			return
		}
	}
}

// printresult prints execution output to standard log
func (c *Client) printResult(resp *protocol.Response) {
	logger.Info("=== action execution successful ===")
	logger.Info("status:    %s", resp.Status)
	logger.Info("action:    %s", resp.Action)
	logger.Info("container: %s", resp.Container)
	logger.Info("message:   %s", resp.Message)
	if len(resp.Details) > 0 {
		logger.Info("details:")
		for k, v := range resp.Details {
			logger.Info("  - %s: %s", k, v)
		}
	}
}
