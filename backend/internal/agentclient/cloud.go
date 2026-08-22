package agentclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type CloudClient struct {
	baseURL string
	http    *http.Client
}

func NewCloudClient(baseURL string) *CloudClient {
	return &CloudClient{baseURL: strings.TrimRight(baseURL, "/"), http: newAgentHTTPClient()}
}

func newAgentHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (client *CloudClient) Enroll(ctx context.Context, userPublicID, name string, identity *DeviceIdentity, prove func(context.Context, string) (string, error)) (string, error) {
	var challenge struct {
		ChallengeID string `json:"challengeId"`
		Canonical   string `json:"canonical"`
		ExpiresAt   string `json:"expiresAt"`
	}
	if err := client.post(ctx, "/api/v1/agent/bridge/enrollment-challenges", map[string]any{
		"userPublicID": userPublicID, "name": name, "platform": runtime.GOOS, "publicKey": identity.PublicKeyBase64URL(),
	}, &challenge); err != nil {
		return "", err
	}
	if !validPublicID(challenge.ChallengeID, "age") || !strings.HasPrefix(challenge.Canonical, "deeix-device-enrollment-v1\n") {
		return "", errors.New("server returned an invalid enrollment challenge")
	}
	proof, err := prove(ctx, challenge.Canonical)
	if err != nil {
		return "", err
	}
	signature, err := identity.SignBase64URL(challenge.Canonical)
	if err != nil {
		return "", err
	}
	var result struct {
		DeviceID string `json:"deviceId"`
		Status   string `json:"status"`
	}
	if err = client.post(ctx, "/api/v1/agent/bridge/enrollments", map[string]any{
		"challengeId": challenge.ChallengeID, "proof": proof, "signature": signature,
	}, &result); err != nil {
		return "", err
	}
	if !validPublicID(result.DeviceID, "agd") || result.Status != "active" {
		return "", errors.New("server returned an invalid device enrollment")
	}
	return result.DeviceID, nil
}

func (client *CloudClient) ConnectionToken(ctx context.Context, config Config, identity *DeviceIdentity) (string, error) {
	var challenge struct {
		ChallengeID string `json:"challengeId"`
		Challenge   string `json:"challenge"`
		ExpiresAt   string `json:"expiresAt"`
	}
	if err := client.post(ctx, "/api/v1/agent/bridge/token-challenges", map[string]any{"deviceId": config.DeviceID}, &challenge); err != nil {
		return "", err
	}
	if !validPublicID(challenge.ChallengeID, "agc") || !strings.HasPrefix(challenge.Challenge, "deeix_challenge_") {
		return "", errors.New("server returned an invalid connection challenge")
	}
	signature, err := identity.SignBase64URL(challenge.Challenge)
	if err != nil {
		return "", err
	}
	var result struct {
		ConnectionToken string `json:"connectionToken"`
		ExpiresAt       string `json:"expiresAt"`
	}
	if err = client.post(ctx, "/api/v1/agent/bridge/tokens", map[string]any{
		"deviceId": config.DeviceID, "challengeId": challenge.ChallengeID, "signature": signature,
	}, &result); err != nil {
		return "", err
	}
	if !strings.HasPrefix(result.ConnectionToken, "deeix_connection_") || len(result.ConnectionToken) > 128 {
		return "", errors.New("server returned an invalid connection token")
	}
	return result.ConnectionToken, nil
}

func (client *CloudClient) DownloadArtifacts(ctx context.Context, commandID string, command AgentCommand, grants []ArtifactGrant, workspaces map[string]Workspace) (map[string]LocalArtifact, error) {
	result := make(map[string]LocalArtifact)
	if command.Kind != "turn.start" && command.Kind != "turn.steer" {
		if len(grants) != 0 {
			return nil, errors.New("gateway command does not accept artifacts")
		}
		return result, nil
	}
	refs := make(map[string]bool)
	for _, input := range command.Input {
		if input.Kind == "artifact" {
			refs[input.ArtifactRef] = true
		}
	}
	if len(refs) != len(grants) {
		return nil, errors.New("artifact grants do not match command input")
	}
	byRef := make(map[string]ArtifactGrant, len(grants))
	for _, grant := range grants {
		byRef[grant.ArtifactRef] = grant
	}
	workspace, ok := workspaces[command.WorkspaceID]
	if !ok {
		return nil, errors.New("artifact workspace is not registered")
	}
	directory, err := resolveWorkspaceSubpath(workspace.Root, ".deeix", "artifacts")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	type downloadResult struct {
		ref      string
		artifact LocalArtifact
		err      error
	}
	results := make(chan downloadResult, len(refs))
	var downloads sync.WaitGroup
	for ref := range refs {
		grant, ok := byRef[ref]
		if !ok {
			return nil, fmt.Errorf("artifact grant is missing: %s", ref)
		}
		extension := filepath.Ext(filepath.Base(grant.FileName))
		if len(extension) > 16 || !artifactExtensionPattern.MatchString(extension) {
			extension = ""
		}
		target := filepath.Join(directory, grant.ArtifactRef+extension)
		if !pathWithin(directory, target) {
			return nil, errors.New("artifact target escapes its directory")
		}
		downloads.Go(func() {
			matches, matchErr := artifactFileMatches(target, grant)
			if matchErr == nil && !matches {
				matchErr = client.downloadArtifact(ctx, commandID, grant, target)
			}
			results <- downloadResult{ref: ref, artifact: LocalArtifact{Path: target, MimeType: grant.MimeType}, err: matchErr}
		})
	}
	downloads.Wait()
	close(results)
	for item := range results {
		if item.err != nil {
			return nil, item.err
		}
		result[item.ref] = item.artifact
	}
	return result, nil
}

func artifactFileMatches(path string, grant ArtifactGrant) (bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Size() != grant.SizeBytes {
		return false, nil
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return false, err
	}
	return strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), grant.SHA256), nil
}

func (client *CloudClient) downloadArtifact(ctx context.Context, commandID string, grant ArtifactGrant, target string) error {
	endpoint := fmt.Sprintf("%s/api/v1/agent/bridge/artifacts/%s/content?command=%s&expires=%s", client.baseURL,
		url.PathEscape(grant.ArtifactRef), url.QueryEscape(commandID), url.QueryEscape(grant.ExpiresAt))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer deeix_artifact_"+grant.Grant)
	response, err := client.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("artifact download failed (%d)", response.StatusCode)
	}
	temporary := target + ".partial"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, grant.SizeBytes+1))
	syncErr := file.Sync()
	closeErr := file.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || written != grant.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != grant.SHA256 {
		_ = os.Remove(temporary)
		if copyErr != nil {
			return copyErr
		}
		return errors.New("artifact integrity check failed")
	}
	_ = os.Remove(target)
	if err = os.Rename(temporary, target); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func (client *CloudClient) post(ctx context.Context, path string, input any, output any) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	var envelope struct {
		ErrorMsg string          `json:"errorMsg"`
		Data     json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return fmt.Errorf("server returned an invalid response (%d)", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || envelope.ErrorMsg != "" {
		message := strings.TrimSpace(envelope.ErrorMsg)
		if message == "" {
			message = response.Status
		}
		return fmt.Errorf("server request failed: %s", message)
	}
	if len(envelope.Data) == 0 || json.Unmarshal(envelope.Data, output) != nil {
		return errors.New("server response data is invalid")
	}
	return nil
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func resolveWorkspaceSubpath(root string, parts ...string) (string, error) {
	target := filepath.Join(append([]string{root}, parts...)...)
	cursor := target
	missing := make([]string, 0, len(parts))
	for {
		_, err := os.Stat(cursor)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cursor)
		if parent == cursor {
			return "", errors.New("workspace path has no existing parent")
		}
		missing = append([]string{filepath.Base(cursor)}, missing...)
		cursor = parent
	}
	resolved, err := filepath.EvalSymlinks(cursor)
	if err != nil {
		return "", err
	}
	resolved = filepath.Join(append([]string{resolved}, missing...)...)
	if !pathWithin(root, resolved) {
		return "", errors.New("workspace path escapes the registered root")
	}
	return resolved, nil
}
