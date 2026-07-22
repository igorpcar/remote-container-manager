package client

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"remote-container-manager/pkg/auth"
)

// loadkeys resolves public and private key from file or raw command-line string
func LoadKeys(pubInput, privInput string) (string, ed25519.PrivateKey, error) {
	if pubInput == "" || privInput == "" {
		return "", nil, fmt.Errorf("both -pubkey and -privkey must be provided")
	}

	// resolve public key input (filepath or raw string)
	pubStr := strings.TrimSpace(readInputOrFile(pubInput))
	pubBytes, err := auth.DecodeKey(pubStr)
	if err != nil {
		return "", nil, fmt.Errorf("invalid public key: %w", err)
	}

	// resolve private key input (filepath or raw string)
	privStr := strings.TrimSpace(readInputOrFile(privInput))
	privBytes, err := auth.DecodeKey(privStr)
	if err != nil {
		return "", nil, fmt.Errorf("invalid private key: %w", err)
	}

	// ed25519 private key length check (32 seed bytes or 64 key bytes)
	var privKey ed25519.PrivateKey
	if len(privBytes) == ed25519.SeedSize {
		privKey = ed25519.NewKeyFromSeed(privBytes)
	} else if len(privBytes) == ed25519.PrivateKeySize {
		privKey = ed25519.PrivateKey(privBytes)
	} else {
		return "", nil, fmt.Errorf("invalid ed25519 private key size: %d", len(privBytes))
	}

	pubKeyHex := hex.EncodeToString(pubBytes)
	return pubKeyHex, privKey, nil
}

// readinputorfile returns file content if path exists, otherwise returns input string
func readInputOrFile(input string) string {
	content, err := os.ReadFile(input)
	if err == nil {
		return string(content)
	}
	return input
}
