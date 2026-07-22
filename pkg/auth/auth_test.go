package auth

import (
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"
)

func TestAuthManager_KeyAuthorization(t *testing.T) {
	// create auth manager with 10s ttl
	mgr := NewManager(10 * time.Second)

	pubKey, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}

	pubHex := hex.EncodeToString(pubKey)

	// test unauthorized key
	if mgr.IsAuthorized(pubHex) {
		t.Errorf("key should not be authorized yet")
	}

	// load authorized key
	mgr.LoadAuthorizedKeys([]string{pubHex})

	// test authorized key
	if !mgr.IsAuthorized(pubHex) {
		t.Errorf("key should be authorized after loading")
	}
}

func TestAuthManager_ChallengeResponseFlow(t *testing.T) {
	mgr := NewManager(10 * time.Second)

	pubKey, privKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}

	pubHex := hex.EncodeToString(pubKey)
	mgr.LoadAuthorizedKeys([]string{pubHex})

	// generate challenge
	ch, err := mgr.GenerateChallenge(pubHex, "start")
	if err != nil {
		t.Fatalf("failed to generate challenge: %v", err)
	}

	// sign challenge
	sigB64 := SignChallenge(privKey, ch.Data)

	// verify valid signature
	chVerified, err := mgr.VerifyChallenge(ch.ID, pubHex, sigB64)
	if err != nil {
		t.Fatalf("failed to verify valid challenge: %v", err)
	}

	if chVerified.Action != "start" {
		t.Errorf("expected action 'start', got '%s'", chVerified.Action)
	}

	// verify challenge cannot be reused (replay protection)
	_, err = mgr.VerifyChallenge(ch.ID, pubHex, sigB64)
	if err == nil {
		t.Errorf("reusing challenge should fail")
	}
}

func TestAuthManager_InvalidSignature(t *testing.T) {
	mgr := NewManager(10 * time.Second)

	pubKey1, _, _ := GenerateKeyPair()
	_, privKey2, _ := GenerateKeyPair()

	pubHex1 := hex.EncodeToString(pubKey1)
	mgr.LoadAuthorizedKeys([]string{pubHex1})

	// generate challenge for pubkey1
	ch, err := mgr.GenerateChallenge(pubHex1, "status")
	if err != nil {
		t.Fatalf("failed to generate challenge: %v", err)
	}

	// sign with different key privkey2
	badSigB64 := SignChallenge(privKey2, ch.Data)

	// verification should fail
	_, err = mgr.VerifyChallenge(ch.ID, pubHex1, badSigB64)
	if err == nil {
		t.Errorf("verification with wrong private key should fail")
	}
}

func TestAuthManager_InvalidSignatureData(t *testing.T) {
	mgr := NewManager(10 * time.Second)

	pubKey, privKey, _ := GenerateKeyPair()
	pubHex := hex.EncodeToString(pubKey)
	mgr.LoadAuthorizedKeys([]string{pubHex})

	ch, _ := mgr.GenerateChallenge(pubHex, "stop")

	// tamper with signature
	sigB64 := SignChallenge(privKey, ch.Data)
	tamperedSigBytes, _ := base64.StdEncoding.DecodeString(sigB64)
	tamperedSigBytes[0] ^= 0xFF
	tamperedSigB64 := base64.StdEncoding.EncodeToString(tamperedSigBytes)

	_, err := mgr.VerifyChallenge(ch.ID, pubHex, tamperedSigB64)
	if err == nil {
		t.Errorf("tampered signature verification should fail")
	}
}
