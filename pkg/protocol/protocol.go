package protocol

// message types for client-server nats communication
const (
	TypeInitChallenge   = "init_challenge"
	TypeChallengeResp   = "challenge_response"
	TypeVerifyChallenge = "verify_challenge"
	TypeHeartbeat       = "heartbeat"
	TypeResult          = "result"
	TypeError           = "error"
)

// actions supported for docker container management
const (
	ActionStart  = "start"
	ActionStop   = "stop"
	ActionStatus = "status"
)

// request payload sent by client to server
type Request struct {
	Type        string `json:"type"`
	Action      string `json:"action,omitempty"`
	PublicKey   string `json:"public_key"`
	ChallengeID string `json:"challenge_id,omitempty"`
	Signature   string `json:"signature,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	Heartbeat   bool   `json:"heartbeat,omitempty"`
}

// response payload sent by server to client
type Response struct {
	Type              string            `json:"type"`
	Status            string            `json:"status"`
	Action            string            `json:"action,omitempty"`
	Container         string            `json:"container,omitempty"`
	ChallengeID       string            `json:"challenge_id,omitempty"`
	ChallengeData     string            `json:"challenge_data,omitempty"`
	SessionID         string            `json:"session_id,omitempty"`
	HeartbeatInterval int               `json:"heartbeat_interval,omitempty"`
	Error             string            `json:"error,omitempty"`
	Message           string            `json:"message,omitempty"`
	Details           map[string]string `json:"details,omitempty"`
}
