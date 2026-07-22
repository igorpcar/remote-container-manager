package protocol

import (
	"encoding/json"
	"testing"
)

func TestProtocol_RequestJSON(t *testing.T) {
	req := Request{
		Type:      TypeInitChallenge,
		Action:    ActionStart,
		PublicKey: "abc123def456",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	var decoded Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	if decoded.Type != TypeInitChallenge || decoded.Action != ActionStart || decoded.PublicKey != "abc123def456" {
		t.Errorf("decoded request does not match expected values")
	}
}

func TestProtocol_ResponseJSON(t *testing.T) {
	resp := Response{
		Type:        TypeChallengeResp,
		Status:      "pending_challenge",
		ChallengeID: "ch-12345",
		Details:     map[string]string{"foo": "bar"},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}

	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if decoded.ChallengeID != "ch-12345" || decoded.Details["foo"] != "bar" {
		t.Errorf("decoded response does not match expected values")
	}
}
