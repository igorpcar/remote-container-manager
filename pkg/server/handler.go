package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"remote-container-manager/pkg/auth"
	"remote-container-manager/pkg/docker"
	"remote-container-manager/pkg/logger"
	"remote-container-manager/pkg/protocol"

	"github.com/nats-io/nats.go"
)

// handleincomingmsg processes incoming json requests over nats
func handleIncomingMsg(nc *nats.Conn, msg *nats.Msg, authMgr *auth.Manager, dockerMgr *docker.Manager, sessionMgr *SessionManager, targetContainer string, trace bool) {
	// trace incoming nats message if trace is enabled
	if trace {
		logger.TraceIn(msg.Subject, msg.Reply, msg.Data)
	}

	var req protocol.Request
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		sendResponse(nc, msg, &protocol.Response{
			Type:   protocol.TypeError,
			Status: "error",
			Error:  fmt.Sprintf("invalid json payload: %v", err),
		}, trace)
		return
	}

	switch req.Type {
	case protocol.TypeInitChallenge:
		handleInitChallenge(nc, msg, req, authMgr, targetContainer, trace)
	case protocol.TypeVerifyChallenge:
		handleVerifyChallenge(nc, msg, req, authMgr, dockerMgr, sessionMgr, targetContainer, trace)
	case protocol.TypeHeartbeat:
		handleHeartbeat(nc, msg, req, sessionMgr, trace)
	default:
		sendResponse(nc, msg, &protocol.Response{
			Type:   protocol.TypeError,
			Status: "error",
			Error:  fmt.Sprintf("unsupported request type: %s", req.Type),
		}, trace)
	}
}

// handleinitchallenge validates public key authorization and generates a challenge
func handleInitChallenge(nc *nats.Conn, msg *nats.Msg, req protocol.Request, authMgr *auth.Manager, targetContainer string, trace bool) {
	// check if client public key is authorized
	if !authMgr.IsAuthorized(req.PublicKey) {
		logger.Error("unauthorized access attempt for container %s with key %s", targetContainer, req.PublicKey)
		sendResponse(nc, msg, &protocol.Response{
			Type:      protocol.TypeError,
			Status:    "unauthorized",
			Container: targetContainer,
			Error:     "public key is not authorized to manage this container",
		}, trace)
		return
	}

	// validate requested action
	if req.Action != protocol.ActionStart && req.Action != protocol.ActionStop && req.Action != protocol.ActionStatus {
		sendResponse(nc, msg, &protocol.Response{
			Type:   protocol.TypeError,
			Status: "error",
			Error:  fmt.Sprintf("invalid action: %s", req.Action),
		}, trace)
		return
	}

	// generate challenge
	challenge, err := authMgr.GenerateChallenge(req.PublicKey, req.Action)
	if err != nil {
		sendResponse(nc, msg, &protocol.Response{
			Type:   protocol.TypeError,
			Status: "error",
			Error:  fmt.Sprintf("failed to generate challenge: %v", err),
		}, trace)
		return
	}

	logger.Info("generated challenge %s for public key %s (action: %s)", challenge.ID, req.PublicKey, req.Action)

	// respond with challenge details
	sendResponse(nc, msg, &protocol.Response{
		Type:          protocol.TypeChallengeResp,
		Status:        "pending_challenge",
		Container:     targetContainer,
		ChallengeID:   challenge.ID,
		ChallengeData: base64.StdEncoding.EncodeToString(challenge.Data),
	}, trace)
}

// handleverifychallenge verifies signed challenge and executes container action
func handleVerifyChallenge(nc *nats.Conn, msg *nats.Msg, req protocol.Request, authMgr *auth.Manager, dockerMgr *docker.Manager, sessionMgr *SessionManager, targetContainer string, trace bool) {
	// verify challenge signature
	challenge, err := authMgr.VerifyChallenge(req.ChallengeID, req.PublicKey, req.Signature)
	if err != nil {
		logger.Error("challenge verification failed for id %s: %v", req.ChallengeID, err)
		sendResponse(nc, msg, &protocol.Response{
			Type:      protocol.TypeError,
			Status:    "forbidden",
			Container: targetContainer,
			Error:     fmt.Sprintf("challenge verification failed: %v", err),
		}, trace)
		return
	}

	logger.Info("challenge %s verified successfully for action: %s on container: %s", challenge.ID, challenge.Action, targetContainer)

	// execute docker action
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	info, err := dockerMgr.ExecuteAction(ctx, challenge.Action, targetContainer)
	if err != nil {
		logger.Error("failed to execute action %s on container %s: %v", challenge.Action, targetContainer, err)
		sendResponse(nc, msg, &protocol.Response{
			Type:      protocol.TypeError,
			Status:    "execution_failed",
			Action:    challenge.Action,
			Container: targetContainer,
			Error:     fmt.Sprintf("docker operation failed: %v", err),
		}, trace)
		return
	}

	// build result details map
	details := map[string]string{
		"id":         info.ID,
		"name":       info.Name,
		"state":      info.State,
		"running":    fmt.Sprintf("%t", info.Running),
		"started_at": info.StartedAt,
	}

	sessionID := ""
	heartbeatInterval := 0

	// create heartbeat session if requested and action is start
	if challenge.Action == protocol.ActionStart && req.Heartbeat {
		sess, err := sessionMgr.CreateSession(targetContainer, req.PublicKey, dockerMgr)
		if err != nil {
			logger.Error("failed to create heartbeat session: %v", err)
		} else {
			sessionID = sess.ID
			heartbeatInterval = 3
			logger.Info("active heartbeat session created: %s (interval: %ds)", sessionID, heartbeatInterval)
		}
	}

	logger.Info("action %s completed successfully for container %s (state: %s)", challenge.Action, targetContainer, info.State)

	sendResponse(nc, msg, &protocol.Response{
		Type:              protocol.TypeResult,
		Status:            "success",
		Action:            challenge.Action,
		Container:         targetContainer,
		SessionID:         sessionID,
		HeartbeatInterval: heartbeatInterval,
		Message:           fmt.Sprintf("action '%s' executed successfully", challenge.Action),
		Details:           details,
	}, trace)
}

// handleheartbeat processes incoming ping to reset watchdog timer
func handleHeartbeat(nc *nats.Conn, msg *nats.Msg, req protocol.Request, sessionMgr *SessionManager, trace bool) {
	if req.SessionID == "" {
		sendResponse(nc, msg, &protocol.Response{
			Type:   protocol.TypeError,
			Status: "error",
			Error:  "missing session_id in heartbeat request",
		}, trace)
		return
	}

	ok := sessionMgr.RefreshHeartbeat(req.SessionID)
	if !ok {
		sendResponse(nc, msg, &protocol.Response{
			Type:   protocol.TypeError,
			Status: "expired",
			Error:  "session not found or expired",
		}, trace)
		return
	}

	sendResponse(nc, msg, &protocol.Response{
		Type:   protocol.TypeHeartbeat,
		Status: "ack",
	}, trace)
}

// sendresponse publishes reply json to client via nats reply subject
func sendResponse(nc *nats.Conn, msg *nats.Msg, resp *protocol.Response, trace bool) {
	data, err := json.Marshal(resp)
	if err != nil {
		logger.Error("error marshaling response: %v", err)
		return
	}

	if msg.Reply != "" {
		if trace {
			logger.TraceOut(msg.Reply, data)
		}
		_ = nc.Publish(msg.Reply, data)
	}
}
