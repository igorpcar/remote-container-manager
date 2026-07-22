package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"

	"remote-container-manager/pkg/auth"
	"remote-container-manager/pkg/logger"
)

func main() {
	// command line flags definition
	outDir := flag.String("out", ".", "directory to save public.key and private.key files")
	flag.Parse()

	// generate ed25519 key pair
	pubKey, privKey, err := auth.GenerateKeyPair()
	if err != nil {
		logger.Error("failed to generate key pair: %v", err)
		os.Exit(1)
	}

	pubHex := hex.EncodeToString(pubKey)
	privHex := hex.EncodeToString(privKey)

	// prepare output paths
	pubPath := filepath.Join(*outDir, "public.key")
	privPath := filepath.Join(*outDir, "private.key")

	// write public key file
	if err := os.WriteFile(pubPath, []byte(pubHex+"\n"), 0644); err != nil {
		logger.Error("failed to write public key file: %v", err)
		os.Exit(1)
	}

	// write private key file with restricted permissions
	if err := os.WriteFile(privPath, []byte(privHex+"\n"), 0600); err != nil {
		logger.Error("failed to write private key file: %v", err)
		os.Exit(1)
	}

	logger.Info("=== ed25519 keypair generated successfully ===")
	logger.Info("public key  (saved to %s): %s", pubPath, pubHex)
	logger.Info("private key (saved to %s): %s", privPath, privHex)
	logger.Info("to authorize this client on the server, add the public key to authorized_keys file or use -key flag.")
}

// verify ed25519 key interface compliance
var _ ed25519.PublicKey = nil
