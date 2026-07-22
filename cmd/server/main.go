package main

import (
	"flag"
	"os"
	"time"

	"remote-container-manager/pkg/logger"
	"remote-container-manager/pkg/server"

	"github.com/nats-io/nats.go"
)

func main() {
	// command line flags definition
	natsURL := flag.String("nats", nats.DefaultURL, "nats server url")
	containerName := flag.String("container", "timescaledb", "target docker container name or id")
	subject := flag.String("topic", "", "nats subject to subscribe (default: container.<container_name>.control)")
	keysFile := flag.String("keys-file", "", "path to authorized public keys file")
	singleKey := flag.String("key", "", "single authorized public key (hex or base64)")
	challengeTTL := flag.Duration("challenge-ttl", 30*time.Second, "challenge expiration timeout")
	heartbeatTimeout := flag.Duration("heartbeat-timeout", 10*time.Second, "heartbeat session watchdog timeout duration")
	trace := flag.Bool("trace", false, "enable nats message tracing (logs sent and received messages)")
	flag.Parse()

	// build server config
	cfg := server.Config{
		NatsURL:          *natsURL,
		ContainerName:    *containerName,
		Subject:          *subject,
		KeysFile:         *keysFile,
		SingleKey:        *singleKey,
		ChallengeTTL:     *challengeTTL,
		HeartbeatTimeout: *heartbeatTimeout,
		Trace:            *trace,
	}

	// create and start server
	srv, err := server.New(cfg)
	if err != nil {
		logger.Error("failed to initialize server: %v", err)
		os.Exit(1)
	}

	if err := srv.Run(); err != nil {
		logger.Error("server error: %v", err)
		os.Exit(1)
	}
}
