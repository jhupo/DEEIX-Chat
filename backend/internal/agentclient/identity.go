package agentclient

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

type storedIdentity struct {
	Version       int    `json:"version"`
	PrivateKeyPEM string `json:"privateKeyPem"`
}

type DeviceIdentity struct {
	privateKey ed25519.PrivateKey
}

func LoadIdentity(path string) (*DeviceIdentity, error) {
	data, err := readFileAtomic(path)
	if err != nil {
		return nil, err
	}
	return parseIdentity(data)
}

func LoadOrCreateIdentity(path string) (*DeviceIdentity, error) {
	identity, err := LoadIdentity(path)
	if err == nil {
		return identity, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	stored := storedIdentity{
		Version:       1,
		PrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})),
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return nil, err
	}
	if err = writeFileAtomic(path, append(data, '\n'), 0o600); err != nil {
		return nil, err
	}
	return &DeviceIdentity{privateKey: privateKey}, nil
}

func parseIdentity(data []byte) (*DeviceIdentity, error) {
	var stored storedIdentity
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&stored); err != nil || requireEOF(decoder) != nil || stored.Version != 1 {
		return nil, errors.New("device identity format is invalid")
	}
	block, rest := pem.Decode([]byte(stored.PrivateKeyPEM))
	if block == nil || block.Type != "PRIVATE KEY" || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("device identity key is invalid")
	}
	value, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse device identity: %w", err)
	}
	privateKey, ok := value.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("device identity is not Ed25519")
	}
	return &DeviceIdentity{privateKey: privateKey}, nil
}

func (identity *DeviceIdentity) PublicKeyBase64URL() string {
	publicKey := identity.privateKey.Public().(ed25519.PublicKey)
	return base64.RawURLEncoding.EncodeToString(publicKey)
}

func (identity *DeviceIdentity) SignBase64URL(value string) (string, error) {
	if value == "" || len(value) > 4096 {
		return "", errors.New("device challenge is invalid")
	}
	signature := ed25519.Sign(identity.privateKey, []byte(value))
	return base64.RawURLEncoding.EncodeToString(signature), nil
}
