package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// challenge struct holds challenge payload and creation time
type Challenge struct {
	ID        string
	PublicKey string
	Action    string
	Data      []byte
	CreatedAt time.Time
}

// manager handles authorization and challenge-response lifecycle
type Manager struct {
	authorizedKeys map[string]bool
	challenges     map[string]*Challenge
	mu             sync.Mutex
	ttl            time.Duration
}

// newmanager creates a new auth manager with specified key whitelist
func NewManager(ttl time.Duration) *Manager {
	return &Manager{
		authorizedKeys: make(map[string]bool),
		challenges:     make(map[string]*Challenge),
		ttl:            ttl,
	}
}

// loadauthorizedkeys loads public keys from a slice of hex or base64 strings
func (m *Manager) LoadAuthorizedKeys(keys []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, k := range keys {
		clean := strings.TrimSpace(k)
		if clean != "" {
			m.authorizedKeys[clean] = true
		}
	}
}

// loadauthorizedkeysfile loads public keys from a line-separated file
func (m *Manager) LoadAuthorizedKeysFile(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read authorized keys file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	m.LoadAuthorizedKeys(lines)
	return nil
}

// isauthorized checks if given public key string is in the whitelist
func (m *Manager) IsAuthorized(pubKeyStr string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	clean := strings.TrimSpace(pubKeyStr)
	return m.authorizedKeys[clean]
}

// generatechallenge creates a random nonce challenge for an authorized client
func (m *Manager) GenerateChallenge(pubKeyStr, action string) (*Challenge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// clean expired challenges
	m.cleanupExpiredLocked()

	// generate 32 random bytes for challenge data
	rawNonce := make([]byte, 32)
	if _, err := rand.Read(rawNonce); err != nil {
		return nil, fmt.Errorf("failed to generate random challenge: %w", err)
	}

	// generate random challenge id
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, fmt.Errorf("failed to generate challenge id: %w", err)
	}

	challengeID := hex.EncodeToString(idBytes)
	challenge := &Challenge{
		ID:        challengeID,
		PublicKey: strings.TrimSpace(pubKeyStr),
		Action:    action,
		Data:      rawNonce,
		CreatedAt: time.Now(),
	}

	m.challenges[challengeID] = challenge
	return challenge, nil
}

// verifychallenge verifies signature for a given challenge id
func (m *Manager) VerifyChallenge(challengeID, pubKeyStr, signatureB64 string) (*Challenge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cleanupExpiredLocked()

	challenge, exists := m.challenges[challengeID]
	if !exists {
		return nil, errors.New("challenge not found or expired")
	}

	if challenge.PublicKey != strings.TrimSpace(pubKeyStr) {
		return nil, errors.New("public key mismatch for challenge")
	}

	// decode public key
	pubKeyBytes, err := DecodeKey(pubKeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid public key format: %w", err)
	}

	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid ed25519 public key length: %d", len(pubKeyBytes))
	}

	// decode signature
	sigBytes, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 signature: %w", err)
	}

	// verify signature against challenge data
	valid := ed25519.Verify(pubKeyBytes, challenge.Data, sigBytes)
	if !valid {
		return nil, errors.New("invalid signature verification failed")
	}

	// delete challenge to prevent replay attacks
	delete(m.challenges, challengeID)

	return challenge, nil
}

// cleanupexpiredlocked removes challenges older than ttl
func (m *Manager) cleanupExpiredLocked() {
	now := time.Now()
	for id, ch := range m.challenges {
		if now.Sub(ch.CreatedAt) > m.ttl {
			delete(m.challenges, id)
		}
	}
}

// generatekeypair creates a new ed25519 keypair
func GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate keypair: %w", err)
	}
	return pub, priv, nil
}

// signchallenge signs challenge raw bytes with private key
func SignChallenge(privKey ed25519.PrivateKey, challengeData []byte) string {
	sig := ed25519.Sign(privKey, challengeData)
	return base64.StdEncoding.EncodeToString(sig)
}

// encodekey encodes byte key to hex string
func EncodeKey(key []byte) string {
	return hex.EncodeToString(key)
}

// decodekey decodes key from hex or base64 string
func DecodeKey(keyStr string) ([]byte, error) {
	clean := strings.TrimSpace(keyStr)

	// try hex decode
	b, err := hex.DecodeString(clean)
	if err == nil {
		return b, nil
	}

	// try base64 decode
	b, err = base64.StdEncoding.DecodeString(clean)
	if err == nil {
		return b, nil
	}

	return nil, errors.New("key string is neither valid hex nor base64")
}
