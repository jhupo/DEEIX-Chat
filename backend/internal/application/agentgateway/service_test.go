package agentgateway

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	domainagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/agentgateway"
)

func TestCredentialDerivationAndDeviceSignature(t *testing.T) {
	service := &Service{secret: []byte("01234567890123456789012345678901")}
	deviceID := uint(9)
	credential := domainagent.Credential{
		PublicID: "agc_0123456789abcdef0123456789abcdef", UserID: 7, DeviceID: &deviceID,
		Kind: domainagent.CredentialKindChallenge, DerivationKeyVersion: 1,
		DeviceCredentialVersion: 2, ExpiresAt: time.Unix(123456, 0).UTC(),
	}
	first := service.deriveBearer(&credential)
	second := service.deriveBearer(&credential)
	if first != second || first == "" || hashBearer(first) == first {
		t.Fatalf("credential derivation is not stable and one-way hashed")
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(privateKey, []byte(first))
	if !ed25519.Verify(publicKey, []byte(first), signature) {
		t.Fatal("valid device signature rejected")
	}
	if ed25519.Verify(publicKey, []byte(first+"x"), signature) {
		t.Fatal("modified challenge signature accepted")
	}
	encodedKey := base64.RawURLEncoding.EncodeToString(publicKey)
	decodedKey, err := decodePublicKey(encodedKey)
	if err != nil || string(decodedKey) != string(publicKey) {
		t.Fatalf("public key round trip failed: %v", err)
	}
}

func TestAgentPublicIDValidation(t *testing.T) {
	if !validUserPublicID("f6f910e920934def9a5cda479fc25251") {
		t.Fatal("existing user public ID format should be accepted")
	}
	if !validPublicID("agd_f6f910e920934def9a5cda479fc25251", "agd") {
		t.Fatal("device public ID format should be accepted")
	}
	for _, value := range []string{"", "agd_123", "agt_f6f910e920934def9a5cda479fc25251"} {
		if validPublicID(value, "agd") {
			t.Fatalf("invalid device public ID accepted: %q", value)
		}
	}
}

func TestRuntimeChallengeBindsExistingUserPublicID(t *testing.T) {
	expiresAt := time.Unix(123456, 0).UTC()
	canonical := runtimeChallengeCanonical(
		"f6f910e920934def9a5cda479fc25251",
		"agd_f6f910e920934def9a5cda479fc25251",
		"default",
		"fingerprint",
		"nonce",
		expiresAt,
	)
	want := "deeix-runtime-auth-proof-v1\nf6f910e920934def9a5cda479fc25251\nagd_f6f910e920934def9a5cda479fc25251\ndefault\nfingerprint\nnonce\n123456"
	if canonical != want {
		t.Fatalf("runtime challenge canonical = %q, want %q", canonical, want)
	}
}

func TestAgentWorkPayloadValidation(t *testing.T) {
	validSettingsJSON := json.RawMessage(`{"model":"gpt-5.6","reasoningEffort":"high","approvalPolicy":"on-request","sandboxPolicy":"workspace-write"}`)
	validInputJSON := json.RawMessage(`[{"kind":"text","text":"inspect the repository"},{"kind":"artifact","artifactRef":"agart_0123456789abcdef0123456789abcdef"}]`)
	validResponseJSON := json.RawMessage(`{"kind":"approval","decision":"accept"}`)
	if !validSettings(validSettingsJSON) || !validInput(validInputJSON) || !validInteractionResponse(validResponseJSON) {
		t.Fatal("valid agent work payload rejected")
	}

	invalidSettings := []json.RawMessage{
		json.RawMessage(`{"protocol":"responses"}`),
		json.RawMessage(`{"reasoningEffort":"extreme"}`),
		json.RawMessage(`[]`),
	}
	for _, value := range invalidSettings {
		if validSettings(value) {
			t.Fatalf("invalid settings accepted: %s", value)
		}
	}

	invalidInputs := []json.RawMessage{
		json.RawMessage(`[]`),
		json.RawMessage(`[{"kind":"artifact","path":"C:/secret"}]`),
		json.RawMessage(`[{"kind":"text","text":"ok","extra":true}]`),
	}
	for _, value := range invalidInputs {
		if validInput(value) {
			t.Fatalf("invalid input accepted: %s", value)
		}
	}

	invalidResponses := []json.RawMessage{
		json.RawMessage(`{"kind":"approval","decision":"maybe"}`),
		json.RawMessage(`{"kind":"permission","decision":"accept","scope":"device"}`),
		json.RawMessage(`{"kind":"unknown"}`),
	}
	for _, value := range invalidResponses {
		if validInteractionResponse(value) {
			t.Fatalf("invalid interaction response accepted: %s", value)
		}
	}
}
