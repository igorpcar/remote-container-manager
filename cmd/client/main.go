package main

import (
	"flag"
	"os"
	"time"

	"remote-container-manager/pkg/client"
	"remote-container-manager/pkg/logger"

	"github.com/nats-io/nats.go"
)

func main() {
	// command line flags definition
	natsURL := flag.String("nats", nats.DefaultURL, "nats server url")
	containerName := flag.String("container", "timescaledb", "target docker container name or id")
	action := flag.String("action", "status", "action to perform: start, stop, or status")
	subject := flag.String("topic", "", "nats subject (default: container.<container_name>.control)")
	privKeyInput := flag.String("privkey", "", "client private key (raw hex/base64 string or filepath)")
	pubKeyInput := flag.String("pubkey", "", "client public key (raw hex/base64 string or filepath)")
	reqTimeout := flag.Duration("timeout", 10*time.Second, "nats request timeout")
	trace := flag.Bool("trace", false, "enable nats message tracing (logs sent and received messages)")
	heartbeat := flag.Bool("heartbeat", false, "enable active heartbeat monitoring for start action (stops container when client exits)")
	flag.Parse()

	// build client config
	cfg := client.Config{
		NatsURL:       *natsURL,
		ContainerName: *containerName,
		Action:        *action,
		Subject:       *subject,
		PubKeyInput:   *pubKeyInput,
		PrivKeyInput:  *privKeyInput,
		Timeout:       *reqTimeout,
		Trace:         *trace,
		Heartbeat:     *heartbeat,
	}

	// create and run client
	c, err := client.New(cfg)
	if err != nil {
		logger.Error("failed to initialize client: %v", err)
		os.Exit(1)
	}

	if err := c.ExecuteAction(); err != nil {
		logger.Error("client execution failed: %v", err)
		os.Exit(1)
	}
}
