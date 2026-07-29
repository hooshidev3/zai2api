// Package server — AES-256-GCM encryption for proxy passwords.
//
// PROXY_ENCRYPTION_KEY must be a 64-char hex string (32 bytes).
// Generate with: openssl rand -hex 32
package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// encryptPassword encrypts a plaintext password using AES-256-GCM.
// Returns "enc:<base64>" on success. Refuses to encrypt if
// PROXY_ENCRYPTION_KEY is not set (no plaintext fallback).
func encryptPassword(plaintext string) (string, error) {
	keyHex := os.Getenv("PROXY_ENCRYPTION_KEY")
	if keyHex == "" {
		return "", fmt.Errorf("PROXY_ENCRYPTION_KEY not set — refusing to store proxy password")
	}

	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return "", fmt.Errorf("PROXY_ENCRYPTION_KEY must be valid hex: %w", err)
	}
	if len(keyBytes) != 32 {
		return "", fmt.Errorf("PROXY_ENCRYPTION_KEY must decode to 32 bytes (got %d)", len(keyBytes))
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "enc:" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptPassword decrypts a password stored with encryptPassword.
// Strings without "enc:" prefix are returned as-is (backward compat).
func decryptPassword(stored string) (string, error) {
	if !strings.HasPrefix(stored, "enc:") {
		return stored, nil
	}

	keyHex := os.Getenv("PROXY_ENCRYPTION_KEY")
	if keyHex == "" {
		return "", fmt.Errorf("PROXY_ENCRYPTION_KEY not set — cannot decrypt")
	}

	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, "enc:"))
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
