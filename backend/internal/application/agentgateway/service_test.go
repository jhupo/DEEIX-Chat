package agentgateway

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
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

func TestNormalizeBridgeJSONReplacesPostgresNUL(t *testing.T) {
	normalized, err := normalizeBridgeJSON(json.RawMessage(`{"kind":"item/completed","payload":{"output":"before\u0000after"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(normalized), `\u0000`) {
		t.Fatalf("PostgreSQL-incompatible NUL was retained: %s", normalized)
	}
	var value struct {
		Payload struct {
			Output string `json:"output"`
		} `json:"payload"`
	}
	if err = json.Unmarshal(normalized, &value); err != nil || value.Payload.Output != "before\uFFFDafter" {
		t.Fatalf("normalized payload = %q, %v", value.Payload.Output, err)
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

func TestEnrollmentChallengeBindsPublicIdentityAndDevice(t *testing.T) {
	expiresAt := time.Unix(123456, 0).UTC()
	canonical := enrollmentChallengeCanonical(
		"f6f910e920934def9a5cda479fc25251",
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"windows",
		"nonce",
		expiresAt,
	)
	want := "deeix-device-enrollment-v1\nf6f910e920934def9a5cda479fc25251\n0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\nwindows\nnonce\n123456"
	if canonical != want {
		t.Fatalf("enrollment challenge canonical = %q, want %q", canonical, want)
	}
}

func TestAgentWorkPayloadValidation(t *testing.T) {
	validSettingsJSON := json.RawMessage(`{"model":"gpt-5.6","reasoningEffort":"high","approvalPolicy":"on-request","approvalsReviewer":"auto_review","sandboxPolicy":"workspace-write"}`)
	validInputJSON := json.RawMessage(`[{"kind":"text","text":"inspect the repository"},{"kind":"skill","resourceRef":"skill_0123456789abcdef0123456789abcdef"},{"kind":"app-mention","resourceRef":"app_0123456789abcdef0123456789abcdef"},{"kind":"artifact","artifactRef":"agart_0123456789abcdef0123456789abcdef"}]`)
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
		json.RawMessage(`[{"kind":"skill","resourceRef":"../../secret"}]`),
	}
	for _, value := range invalidInputs {
		if validInput(value) {
			t.Fatalf("invalid input accepted: %s", value)
		}
	}

	invalidResponses := []json.RawMessage{
		json.RawMessage(`{"kind":"approval","decision":"maybe"}`),
		json.RawMessage(`{"kind":"permission","decision":"accept","scope":"device"}`),
		json.RawMessage(`{"kind":"permission","decision":"accept","unexpected":true}`),
		json.RawMessage(`{"kind":"mcp-elicitation","decision":"accept","content":{"nested":{"value":1}}}`),
		json.RawMessage(`{"kind":"unknown"}`),
	}
	for _, value := range invalidResponses {
		if validInteractionResponse(value) {
			t.Fatalf("invalid interaction response accepted: %s", value)
		}
	}
}

func TestMCPElicitationResponseAcceptsSchemaScalarsOnly(t *testing.T) {
	valid := json.RawMessage(`{"kind":"mcp-elicitation","decision":"accept","content":{"name":"Ada","count":3,"ratio":0.5,"enabled":true}}`)
	if !validInteractionResponse(valid) {
		t.Fatal("MCP elicitation scalar content was rejected")
	}
	for _, invalid := range []json.RawMessage{
		json.RawMessage(`{"kind":"mcp-elicitation","decision":"decline","content":{"name":"Ada"}}`),
		json.RawMessage(`{"kind":"mcp-elicitation","decision":"accept","content":{"items":[1,2]}}`),
		json.RawMessage(`{"kind":"mcp-elicitation","decision":"accept","content":{"value":null}}`),
	} {
		if validInteractionResponse(invalid) {
			t.Fatalf("invalid MCP elicitation response accepted: %s", invalid)
		}
	}
}

func TestProviderManifestValidation(t *testing.T) {
	valid := json.RawMessage(`{"agentVersion":"0.4.57","provider":"codex","runtimeVersion":"0.151.0","protocolVersion":"0.151.0/stable","schemaHash":"424b204943b18e5ffa52667a2aa397c9950730ec1e49ad767e2a016743990541","commands":["agent.update","thread.create","turn.start"],"resources":{"profile":["models"],"workspace":["sessions"]},"inputKinds":["text"],"threadSettings":{"model":true,"reasoningEffort":["high"],"approvalPolicy":["on-request"],"approvalsReviewer":["user","auto_review"],"sandboxPolicy":["workspace-write","danger-full-access"]},"interactionKinds":["command_approval"]}`)
	if !validProviderManifest(valid, "codex") {
		t.Fatal("valid provider manifest rejected")
	}
	for _, value := range []json.RawMessage{
		json.RawMessage(`{"provider":"claude","runtimeVersion":"1","protocolVersion":"1","schemaHash":"f72b2caa3cbfa4298de9e85c62dda6dfbaf2266ffeb916fed30615ca69ff8c74","commands":["turn.start"],"resources":{"profile":["models"],"workspace":["sessions"]},"inputKinds":["text"],"threadSettings":{"model":true,"reasoningEffort":["high"],"approvalPolicy":["on-request"],"sandboxPolicy":["workspace-write"]},"interactionKinds":["command_approval"]}`),
		json.RawMessage(`{"provider":"codex","runtimeVersion":"1","protocolVersion":"1","schemaHash":"bad","commands":["turn.start"],"resources":{"profile":["models"],"workspace":["sessions"]},"inputKinds":["text"],"threadSettings":{"model":true,"reasoningEffort":["high"],"approvalPolicy":["on-request"],"sandboxPolicy":["workspace-write"]},"interactionKinds":["command_approval"]}`),
		json.RawMessage(`{"provider":"codex","runtimeVersion":"0.150.99","protocolVersion":"0.151.0/stable","schemaHash":"424b204943b18e5ffa52667a2aa397c9950730ec1e49ad767e2a016743990541","commands":["turn.start"],"resources":{"profile":["models"],"workspace":["sessions"]},"inputKinds":["text"],"threadSettings":{"model":true,"reasoningEffort":["high"],"approvalPolicy":["on-request"],"sandboxPolicy":["workspace-write"]},"interactionKinds":["command_approval"]}`),
		json.RawMessage(`{"provider":"codex","runtimeVersion":"not-semver","protocolVersion":"0.151.0/stable","schemaHash":"424b204943b18e5ffa52667a2aa397c9950730ec1e49ad767e2a016743990541","commands":["turn.start"],"resources":{"profile":["models"],"workspace":["sessions"]},"inputKinds":["text"],"threadSettings":{"model":true,"reasoningEffort":["high"],"approvalPolicy":["on-request"],"sandboxPolicy":["workspace-write"]},"interactionKinds":["command_approval"]}`),
		json.RawMessage(`{"provider":"codex","runtimeVersion":"0.151.0","protocolVersion":"0.151.0/stable","schemaHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","commands":["turn.start"],"resources":{"profile":["models"],"workspace":["sessions"]},"inputKinds":["text"],"threadSettings":{"model":true,"reasoningEffort":["high"],"approvalPolicy":["on-request"],"sandboxPolicy":["workspace-write"]},"interactionKinds":["command_approval"]}`),
		json.RawMessage(`{"provider":"codex","runtimeVersion":"1","protocolVersion":"1","schemaHash":"f72b2caa3cbfa4298de9e85c62dda6dfbaf2266ffeb916fed30615ca69ff8c74","commands":["raw.command"],"resources":{"profile":["models"],"workspace":["sessions"]},"inputKinds":["text"],"threadSettings":{"model":true,"reasoningEffort":["high"],"approvalPolicy":["on-request"],"sandboxPolicy":["workspace-write"]},"interactionKinds":["command_approval"]}`),
		json.RawMessage(`{"agentVersion":"0.4.x","provider":"codex","runtimeVersion":"1","protocolVersion":"1","schemaHash":"f72b2caa3cbfa4298de9e85c62dda6dfbaf2266ffeb916fed30615ca69ff8c74","commands":["agent.update"],"resources":{"profile":["models"],"workspace":["sessions"]},"inputKinds":["text"],"threadSettings":{"model":true,"reasoningEffort":["high"],"approvalPolicy":["on-request"],"sandboxPolicy":["workspace-write"]},"interactionKinds":["command_approval"]}`),
	} {
		if validProviderManifest(value, "codex") {
			t.Fatalf("invalid provider manifest accepted: %s", value)
		}
	}
}

func TestCodexRuntimeVersionRange(t *testing.T) {
	for _, version := range []string{"0.151.0", "0.151.1-alpha.1", "0.151.1", "0.151.99"} {
		if !validCodexRuntimeVersion(version) {
			t.Fatalf("supported Codex runtime version rejected: %s", version)
		}
	}
	for _, version := range []string{"0.150.99", "0.151.0-rc.1", "0.152.0-alpha.1", "0.152.0", "invalid"} {
		if validCodexRuntimeVersion(version) {
			t.Fatalf("unsupported Codex runtime version accepted: %s", version)
		}
	}
}

func TestAgentVersionComparison(t *testing.T) {
	for _, test := range []struct {
		left, right string
		want        int
	}{{"0.4.56", "0.4.57", -1}, {"0.10.0", "0.9.9", 1}, {"1.0.0", "1.0.0", 0}} {
		if got := compareAgentVersions(test.left, test.right); got != test.want {
			t.Fatalf("compareAgentVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestRuntimeProfileResourceCapabilities(t *testing.T) {
	profile := &domainagent.RuntimeProfile{ManifestJSON: `{"resources":{"profile":["apps"],"workspace":["sessions","skills"]}}`}
	for _, target := range []struct{ scope, name string }{{"profile", "apps"}, {"workspace", "sessions"}, {"workspace", "skills"}} {
		if !runtimeProfileHasResource(profile, target.scope, target.name) {
			t.Fatalf("missing resource capability: %s/%s", target.scope, target.name)
		}
	}
	if runtimeProfileHasResource(profile, "profile", "skills") || runtimeProfileHasResource(profile, "workspace", "apps") {
		t.Fatal("resource capability crossed manifest scope")
	}
}

func TestWorkspaceRevisionIsOptionalAndBounded(t *testing.T) {
	for _, value := range []string{"", "0123456789abcdef01234567"} {
		if !validWorkspaceRevision(value) {
			t.Fatalf("valid workspace revision rejected: %q", value)
		}
	}
	for _, value := range []string{"0123456789abcdef0123456", "0123456789abcdef012345678", "0123456789abcdef0123456g"} {
		if validWorkspaceRevision(value) {
			t.Fatalf("invalid workspace revision accepted: %q", value)
		}
	}
}

func TestSessionSnapshotPayloadValidation(t *testing.T) {
	valid := `{"workspaceId":"workspace-main","revision":"0123456789abcdef01234567","data":[{"sourceThreadRef":"thread_0123456789abcdef0123456789abcdef","preview":"preview","name":"session","modelProvider":"openai","status":"active","createdAt":1,"updatedAt":2,"recencyAt":3,"historyLoaded":false}]}`
	if !validSessionSnapshotPayload(json.RawMessage(valid)) {
		t.Fatal("valid session snapshot payload was rejected")
	}
	for _, invalid := range []string{
		strings.Replace(valid, `"historyLoaded":false`, `"historyLoaded":false,"unknown":true`, 1),
		strings.Replace(valid, "0123456789abcdef01234567", "0123456789ABCDEF01234567", 1),
		strings.Replace(valid, `"historyLoaded":false`, `"historyLoaded":true`, 1),
		strings.Replace(valid, `"data":[`, `"data":null,"ignored":[`, 1),
		strings.Replace(valid, `}]}`, `},{"sourceThreadRef":"thread_0123456789abcdef0123456789abcdef","preview":"","name":"","modelProvider":"","status":"active","createdAt":1,"updatedAt":1,"recencyAt":1,"historyLoaded":false}]}`, 1),
	} {
		if validSessionSnapshotPayload(json.RawMessage(invalid)) {
			t.Fatalf("invalid session snapshot payload was accepted: %s", invalid)
		}
	}
}
