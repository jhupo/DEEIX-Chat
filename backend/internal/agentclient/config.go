package agentclient

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
)

const configVersion = 1

type Config struct {
	Version         int         `json:"version"`
	CloudURL        string      `json:"cloudUrl"`
	UserPublicID    string      `json:"userPublicID"`
	DeviceID        string      `json:"deviceId"`
	ProfileID       string      `json:"profileId"`
	CodexExecutable string      `json:"codexExecutable"`
	Workspaces      []Workspace `json:"workspaces"`
}

type Workspace struct {
	WorkspaceID  string   `json:"workspaceId"`
	Root         string   `json:"root"`
	Name         string   `json:"name"`
	Registered   bool     `json:"registered,omitempty"`
	SessionRoots []string `json:"-"`
}

func DefaultDataDir() (string, error) {
	if value := strings.TrimSpace(os.Getenv("DEEIX_AGENT_DATA_DIR")); value != "" {
		return filepath.Abs(value)
	}
	if runtime.GOOS == "windows" {
		base := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if base == "" {
			return "", errors.New("LOCALAPPDATA is not set")
		}
		return filepath.Join(base, "DEEIX", "Agent"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "DEEIX", "Agent"), nil
	}
	return filepath.Join(home, ".local", "share", "deeix-agent"), nil
}

func NormalizeCloudURL(value string) (string, error) {
	if len(value) == 0 || len(value) > 2048 {
		return "", errors.New("server URL is invalid")
	}
	if err := security.ValidateTrustedOutboundHTTPURL(value); err != nil {
		return "", errors.New("server URL is invalid")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", errors.New("server URL is invalid")
	}
	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
			return "", errors.New("server URL must use HTTPS")
		}
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("server URL must not contain a query or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return strings.TrimRight(parsed.String(), "/"), nil
}

func LoadConfig(path string) (Config, error) {
	data, err := readFileAtomic(path)
	if err != nil {
		return Config{}, err
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("read agent config: %w", err)
	}
	if err = requireEOF(decoder); err != nil {
		return Config{}, fmt.Errorf("read agent config: %w", err)
	}
	if err = config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func SaveConfig(path string, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'), 0o600)
}

func (config Config) Validate() error {
	if config.Version != configVersion {
		return errors.New("agent config version is unsupported")
	}
	normalized, err := NormalizeCloudURL(config.CloudURL)
	if err != nil || normalized != config.CloudURL {
		return errors.New("agent config server URL is invalid")
	}
	if !validHex(config.UserPublicID, 32) {
		return errors.New("agent config user public ID is invalid")
	}
	if !validPublicID(config.DeviceID, "agd") {
		return errors.New("agent config device ID is invalid")
	}
	if !validRef(config.ProfileID, 64) {
		return errors.New("agent config profile ID is invalid")
	}
	if config.CodexExecutable == "" || len(config.CodexExecutable) > 2048 || strings.ContainsRune(config.CodexExecutable, 0) {
		return errors.New("agent config Codex executable is invalid")
	}
	if len(config.Workspaces) > 128 {
		return errors.New("agent config workspaces are invalid")
	}
	seen := make(map[string]struct{}, len(config.Workspaces))
	for _, workspace := range config.Workspaces {
		if !validRef(workspace.WorkspaceID, 64) {
			return errors.New("agent config workspace ID is invalid")
		}
		if !filepath.IsAbs(workspace.Root) || strings.ContainsRune(workspace.Root, 0) {
			return errors.New("agent config workspace root is invalid")
		}
		name := strings.TrimSpace(workspace.Name)
		if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 128 {
			return errors.New("agent config workspace name is invalid")
		}
		if _, ok := seen[workspace.WorkspaceID]; ok {
			return errors.New("agent config workspace IDs must be unique")
		}
		seen[workspace.WorkspaceID] = struct{}{}
	}
	return nil
}

func CanonicalWorkspace(root string) (Workspace, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return Workspace{}, fmt.Errorf("resolve workspace: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return Workspace{}, fmt.Errorf("resolve workspace: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return Workspace{}, errors.New("workspace must be an existing directory")
	}
	sum := sha256.Sum256([]byte(canonical))
	return Workspace{
		WorkspaceID: "workspace-" + hex.EncodeToString(sum[:12]),
		Root:        canonical,
		Name:        filepath.Base(canonical),
	}, nil
}

func codexProjectWorkspace(root string) (Workspace, error) {
	session, err := CanonicalWorkspace(root)
	if err != nil {
		return Workspace{}, err
	}
	projectRoot, ok := gitProjectRoot(session.Root)
	if !ok {
		return Workspace{}, errors.New("workspace is not inside a Git project")
	}
	project, err := CanonicalWorkspace(projectRoot)
	if err != nil {
		return Workspace{}, err
	}
	project.SessionRoots = []string{session.Root}
	return project, nil
}

func gitProjectRoot(root string) (string, bool) {
	for current := root; ; current = filepath.Dir(current) {
		metadataPath := filepath.Join(current, ".git")
		info, err := os.Stat(metadataPath)
		if err == nil {
			if info.IsDir() {
				return current, true
			}
			if info.Mode().IsRegular() {
				if projectRoot, ok := linkedWorktreeProjectRoot(current, metadataPath); ok {
					return projectRoot, true
				}
				return current, true
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
	}
}

func linkedWorktreeProjectRoot(worktreeRoot, metadataPath string) (string, bool) {
	metadata, ok := readSmallGitMetadata(metadataPath)
	if !ok || !strings.HasPrefix(strings.ToLower(metadata), "gitdir:") {
		return "", false
	}
	gitDirectory := strings.TrimSpace(metadata[len("gitdir:"):])
	if gitDirectory == "" || strings.ContainsAny(gitDirectory, "\x00\r\n") {
		return "", false
	}
	if !filepath.IsAbs(gitDirectory) {
		gitDirectory = filepath.Join(worktreeRoot, gitDirectory)
	}
	commonPath, ok := readSmallGitMetadata(filepath.Join(filepath.Clean(gitDirectory), "commondir"))
	if !ok || commonPath == "" || strings.ContainsAny(commonPath, "\x00\r\n") {
		return "", false
	}
	if !filepath.IsAbs(commonPath) {
		commonPath = filepath.Join(gitDirectory, commonPath)
	}
	commonPath, err := filepath.EvalSymlinks(filepath.Clean(commonPath))
	if err != nil || !strings.EqualFold(filepath.Base(commonPath), ".git") {
		return "", false
	}
	projectRoot := filepath.Dir(commonPath)
	info, err := os.Stat(projectRoot)
	return projectRoot, err == nil && info.IsDir()
}

func readSmallGitMetadata(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 4096 {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 4096 {
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".deeix-agent-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err = temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err = temporary.Write(data); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		backup := path + ".previous"
		_ = os.Remove(backup)
		if _, statErr := os.Stat(path); statErr == nil {
			if err = os.Rename(path, backup); err != nil {
				return err
			}
			if err = os.Rename(temporaryPath, path); err != nil {
				_ = os.Rename(backup, path)
				return err
			}
			_ = os.Remove(backup)
			committed = true
			return nil
		}
	}
	if err = os.Rename(temporaryPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}

func readFileAtomic(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if !errors.Is(err, os.ErrNotExist) || runtime.GOOS != "windows" {
		return data, err
	}
	backup := path + ".previous"
	if _, backupErr := os.Stat(backup); backupErr != nil {
		return nil, err
	}
	if restoreErr := os.Rename(backup, path); restoreErr != nil {
		return nil, fmt.Errorf("restore interrupted atomic write: %w", restoreErr)
	}
	return os.ReadFile(path)
}

func requireEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON must contain exactly one value")
		}
		return err
	}
	return nil
}

func validRef(value string, limit int) bool {
	if value == "" || len(value) > limit {
		return false
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || index > 0 && strings.ContainsRune("._:-", character) {
			continue
		}
		return false
	}
	return true
}

func validHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validPublicID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix+"_") && validHex(strings.TrimPrefix(value, prefix+"_"), 32)
}
