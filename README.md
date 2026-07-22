# remote-container-manager

This is a secure application framework designed to remotely start, stop and monitor Docker containers via NATS messaging. It is composed of a client and server application, in which the clients sends a message to the server with a request, authenticate through an Ed25519 public-key chlallenge-response protocol, and execute commands on the server for managing containers. The application is particularly usefull in edge IoT environments with cost optimazation.

Managing containers on remote infrastructure traditionally relies on SSH sessions or opening raw Docker API ports over TLS. This app introduces an outbound communication protocol using NATS, offering the following advantages in real-world production environments:

- No inbound network exposure: server daemons establish outbound connections to a central NATS broker. No open inbound ports (e.g., SSH port 22 or Docker TCP port 2375/2376) are required on target hosts.
- Strict scope: unlike SSH, which grants broad shell access and requires system user account maintenance, this daemon limits remote execution strictly to container lifecycle operations (`start`, `stop`, `status`).
- Cost optimazation and securre operations: ideal for on-demand workloads. Containers can run only while an active client remains connected, if the client crashes or disconnects, the watchdog automatically stops the container to conserve cloud resources or as a security requirement.
- Crypto assymetric auth: uses Ed25519 challenge-response authentication with short-lived nonces, preventing replay attacks and eliminating static API keys, shared secrets, or passwords.

In the example below, the server binary is started for container `timescaledb`. The client sends a request for starting, retrieving status and then stopping the container on the server side.

## 1. Architecture 

The system consists of two primary applications and a cryptographic helper utility:

1. **Server Daemon (`cmd/server`)**:
   - Runs continuously on the target host managing a dedicated Docker container (e.g., `timescaledb`).
   - Subscribes to a container-specific NATS subject (e.g., `container.timescaledb.control`).
   - Verifies incoming client public keys against an authorization whitelist.
   - Generates cryptographically secure challenge nonces.
   - Validates client Ed25519 signatures prior to invoking Docker operations.
   - Maintains an active watchdog timer to automatically stop containers if a client disconnects (when configured to).

2. **Client CLI (`cmd/client`)**:
   - Initiates remote container requests over NATS.
   - Requests challenge nonces from the server using public key identification.
   - Signs challenge nonces locally using its Ed25519 private key.
   - Submits signed responses to trigger execution (`start`, `stop`, `status`).
   - Maintains a background heartbeat ping loop when executing `-action start -heartbeat`.

3. **Key Generator (`cmd/keygen`)**:
   - Utility for generating Ed25519 public and private key pairs (`public.key` and `private.key`).

## 2. Folder structure

```text
.
├── cmd/                        # executables
│   ├── client/                 # client CLI tool for issuing remote container commands
│   │   └── main.go
│   ├── keygen/                 # cryptographic utility for generating Ed25519 key pairs
│   │   └── main.go
│   └── server/                 # server daemon managing target Docker containers via NATS
│       └── main.go
├── pkg/                        # core internal packages and business logic
│   ├── auth/                   # ed25519 public-key authentication & challenge-response logic
│   │   ├── auth.go
│   │   └── auth_test.go
│   ├── client/                 # client controller and key resolution logic
│   │   ├── client.go
│   │   └── keys.go
│   ├── docker/                 # native Docker SDK wrapper 
│   │   └── manager.go
│   ├── logger/                 # colorized structured logger with NATS tracing support
│   │   └── logger.go
│   ├── protocol/               # NATS JSON message schemas and protocol type definitions
│   │   ├── protocol.go
│   │   └── protocol_test.go
│   └── server/                 # NATS server subscription, message handler & watchdog sessions
│       ├── handler.go
│       ├── server.go
│       ├── session.go
│       └── session_test.go
├── docker-compose.yml          # infrastructure setup (NATS broker & sample TimescaleDB container)
├── go.mod                      
├── go.sum                      
└── README.md                   # project documentation
```

## 3. Authentication and heartbeat protocol

```
Client                                              Server
  |                                                   |
  |  1. init_challenge (action, public_key)           |
  |-------------------------------------------------->|
  |                                                   |-- Verify public_key authorization
  |                                                   |-- Generate random challenge nonce
  |  2. challenge_response (challenge_id, nonce)      |
  |<--------------------------------------------------|
  |                                                   |
  |-- Sign nonce using local Ed25519 private_key      |
  |                                                   |
  |  3. verify_challenge (challenge_id, signature)    |
  |-------------------------------------------------->|
  |                                                   |-- Validate Ed25519 signature
  |                                                   |-- Execute Docker API action (start/stop/status)
  |                                                   |-- If action="start" and heartbeat=true:
  |                                                   |   Create SessionID & start Watchdog Timer
  |  4. result (status, session_id, interval)         |
  |<--------------------------------------------------|
  |                                                   |
  |=== Active Heartbeat Loop (when -heartbeat active) =|
  |                                                   |
  |  5. heartbeat ping (session_id) (every 3s)        |
  |-------------------------------------------------->|-- Reset Watchdog Timer
  |  6. heartbeat ack                                 |
  |<--------------------------------------------------|
  |                                                   |
  | (Client application exits or connection drops)    |
  |                                                   |
  | [!] No heartbeat received within 10s              |
  |                                                   |-- Watchdog Timer triggers timeout
  |                                                   |-- Automatically stops container (docker stop)
```

## 3.1 Running the app

### 3.1.1 Prerequisites

- Go 1.22+ installed
- Docker Engine running locally or remotely
- NATS Server instance

### 3.1.2. Build binaries

```bash
go build -o bin/server ./cmd/server
go build -o bin/client ./cmd/client
go build -o bin/keygen ./cmd/keygen
```

### 3.1.3 Launch local NATS Server and sample container (timescaledb)

A standard `docker-compose.yml` configuration is provided for testing:

```bash
docker compose up -d
```

### 3.1.4 Generate ed25519 key pair

```bash
./bin/keygen -out .
```
This generates `public.key` and `private.key` in the local directory.

### 3.1.5. Run server

Start the server daemon specifying the managed container name and authorized client public key:

```bash
./bin/server -container timescaledb -key $(cat public.key) -heartbeat-timeout 10s -trace
```

Server configuration flags:
- `-container`: Name or ID of target Docker container (default: `timescaledb`).
- `-topic`: NATS subject to subscribe to (default: `container.<container_name>.control`).
- `-key`: Authorized public key string (hex or base64).
- `-keys-file`: File path containing authorized public keys (one per line).
- `-heartbeat-timeout`: Timeout threshold before stopping container (default: `10s`).
- `-nats`: NATS server connection URL (default: `nats://localhost:4222`).
- `-trace`: Enables NATS message tracing logs (`<<<<< RECV`, `>>>>> SEND`).

### 3.1.6 Run Client Commands

Obtain container status:
```bash
./bin/client -action status -container timescaledb -pubkey public.key -privkey private.key -trace
```

Start container with active heartbeat monitoring:
```bash
./bin/client -action start -container timescaledb -pubkey public.key -privkey private.key -heartbeat -trace
```

Stop container:
```bash
./bin/client -action stop -container timescaledb -pubkey public.key -privkey private.key -trace
```


## 4. NATS tracing example

When `-trace` is enabled, messages are logged with structured formatting:

```text
2026/07/21 14:55:00 [INF] connecting to nats server at nats://localhost:4222...
2026/07/21 14:55:00 [INF] nats message tracing enabled ([TRC] <<<<< / >>>>>)
2026/07/21 14:55:00 [TRC] >>>>> SEND [container.timescaledb.control] (106 bytes):
  {"type":"init_challenge","action":"start","public_key":"890ddc7f86939b1b2df328be5cdaeffa98eb7cfb6cd720ec7d22625e2c476790","heartbeat":true}
2026/07/21 14:55:00 [TRC] <<<<< RECV [_INBOX.8f2a1...] (148 bytes):
  {"type":"challenge_response","status":"pending_challenge","container":"timescaledb","challenge_id":"a1b2c3d4e5f6","challenge_data":"..."}
2026/07/21 14:55:00 [TRC] >>>>> SEND [container.timescaledb.control] (280 bytes):
  {"type":"verify_challenge","action":"start","public_key":"...","challenge_id":"a1b2c3d4e5f6","signature":"...","heartbeat":true}
2026/07/21 14:55:00 [TRC] <<<<< RECV [_INBOX.8f2a1...] (240 bytes):
  {"type":"result","status":"success","action":"start","container":"timescaledb","session_id":"e4b2a8f1...","heartbeat_interval":3,"message":"action 'start' executed successfully","details":{...}}
2026/07/21 14:55:00 [INF] heartbeat session established! (session_id: e4b2a8f1...)
```


## 5. Unit testing

Run test suites across all packages:

```bash
go test -v ./...
```
