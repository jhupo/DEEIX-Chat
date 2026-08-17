package agentgateway

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	domainagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/agentgateway"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/buildinfo"
	"github.com/google/uuid"
)

var (
	ErrInvalidInput     = errors.New("invalid agent gateway input")
	ErrDeviceNotFound   = errors.New("agent device not found")
	ErrResourceNotFound = errors.New("agent resource not found")
	ErrCredential       = errors.New("invalid or expired agent credential")
	ErrDeviceRevoked    = errors.New("agent device revoked")
	ErrInvalidSignature = errors.New("invalid device signature")
	ErrRuntimeAuth      = errors.New("runtime authentication failed")
	ErrStateConflict    = errors.New("agent resource state conflicts with request")
)

const (
	credentialKeyVersion = uint(1)
	enrollmentTTL        = 10 * time.Minute
	challengeTTL         = 2 * time.Minute
	connectionTTL        = 5 * time.Minute
	runtimeChallengeTTL  = time.Minute
	runtimeLeaseTTL      = 10 * time.Minute
	runtimePresenceTTL   = 75 * time.Second
)

type RuntimeUserResolver interface {
	RuntimeUser(context.Context, uint) (string, int64, error)
	RuntimeUserByPublicID(context.Context, string) (uint, string, int64, error)
}

type RuntimeProofVerifier interface {
	MatchRuntimeProof(context.Context, uint, int64, []byte, []byte) (int64, string, error)
}

type ArtifactContentStore interface {
	OpenAgentArtifact(context.Context, uint, string) (*ArtifactContent, error)
}

type ConversationEventProjector func(context.Context, domainagent.AppliedEventFrame) error

type ArtifactContent struct {
	Reader      io.ReadCloser
	ContentType string
	SizeBytes   int64
}

type Service struct {
	repo      repository.AgentGatewayRepository
	secret    []byte
	now       func() time.Time
	users     RuntimeUserResolver
	proofs    RuntimeProofVerifier
	artifacts ArtifactContentStore
	projector ConversationEventProjector
	notify    func(uint)
}

type DeviceView struct {
	DeviceID           string
	UserID             string
	Name               string
	Platform           string
	AgentVersion       string
	LatestAgentVersion string
	UpdateAvailable    bool
	Status             string
	Online             bool
	LastSeenAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type EnrollmentChallengeResult struct {
	ChallengeID string
	Canonical   string
	ExpiresAt   time.Time
}

type BeginEnrollmentInput struct {
	UserPublicID string
	Name         string
	Platform     string
	PublicKey    string
}

type CompleteEnrollmentInput struct {
	ChallengeID string
	Proof       string
	Signature   string
}

type EnrollDeviceResult struct {
	DeviceID string
	Status   string
}

type ChallengeResult struct {
	ChallengeID string
	Challenge   string
	ExpiresAt   time.Time
}

type ConnectionResult struct {
	ConnectionToken string
	ExpiresAt       time.Time
}

type ConnectionIdentity struct {
	InternalDeviceID   uint
	DeviceID           string
	UserID             uint
	LastAckedServerSeq uint64
	LastAckedBridgeSeq uint64
}

type DeliveryCommand struct {
	InternalID uint
	CommandID  string
	ServerSeq  uint64
	Command    json.RawMessage
	Artifacts  []ArtifactGrant
}

type ArtifactGrant struct {
	ArtifactRef string
	FileName    string
	MimeType    string
	SizeBytes   int64
	SHA256      string
	ExpiresAt   string
	Grant       string
}

type RuntimeChallengeResult struct {
	Profile   *domainagent.RuntimeProfile
	Challenge *domainagent.RuntimeProofChallenge
	Canonical string
}

type RuntimeProfileView struct {
	ProfileID      string
	DeviceID       string
	Provider       string
	Status         string
	VerifiedAt     *time.Time
	LeaseExpiresAt *time.Time
	Manifest       json.RawMessage
}

type WorkspaceView struct {
	WorkspaceID string
	DeviceID    string
	ProfileID   string
	Name        string
	Status      string
	LastSeenAt  time.Time
}

type ArtifactView struct {
	ArtifactID  string
	WorkspaceID string
	FileName    string
	MimeType    string
	SizeBytes   int64
	SHA256      string
}

type ResourceSnapshotView struct {
	Resource    string
	Scope       string
	DeviceID    string
	ProfileID   string
	WorkspaceID string
	Data        json.RawMessage
	RefreshedAt time.Time
}

type ResourceRefreshView struct {
	CommandID string
	Status    string
}

type CommandView struct {
	CommandID    string
	Status       string
	ErrorMessage string
}

type WorkspaceRegistration struct {
	WorkspaceID string
	Name        string
}

type RegisterWorkspaceInput struct {
	DeviceID, ProfileID, Path, IdempotencyKey string
	Create                                    bool
}

type ThreadView struct {
	ThreadID      string
	DeviceID      string
	ProfileID     string
	WorkspaceID   string
	Title         string
	Status        string
	HistoryStatus string
	HistoryError  string
	GitSHA        *string
	GitBranch     *string
	GitOriginURL  *string
	LastEventSeq  uint64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type ThreadHistoryView struct {
	Status string
	Error  string
}

type TurnView struct {
	TurnID    string
	ThreadID  string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type InteractionView struct {
	InteractionID string
	ThreadID      string
	TurnID        string
	RunID         string
	Kind          string
	Status        string
	Request       json.RawMessage
	CreatedAt     time.Time
}

type StartThreadInput struct {
	DeviceID, ProfileID, WorkspaceID string
	ConversationID                   uint
	Title                            string
	Settings                         json.RawMessage
	InitialInput                     json.RawMessage
	InitialRunID                     string
	IdempotencyKey                   string
}

type StartTurnInput struct {
	ThreadID, IdempotencyKey string
	RunID                    string
	Input, Settings          json.RawMessage
}

type RespondInteractionInput struct {
	InteractionID, IdempotencyKey string
	Response                      json.RawMessage
}

type StartThreadResult struct {
	Thread ThreadView
	Turn   *TurnView
}

func NewService(repo repository.AgentGatewayRepository, secret string) (*Service, error) {
	if repo == nil || len(strings.TrimSpace(secret)) < 32 {
		return nil, ErrInvalidInput
	}
	return &Service{repo: repo, secret: []byte(secret), now: time.Now}, nil
}

func (s *Service) SetRuntimeAuth(users RuntimeUserResolver, proofs RuntimeProofVerifier) {
	s.users = users
	s.proofs = proofs
}

func (s *Service) SetArtifactContentStore(store ArtifactContentStore) { s.artifacts = store }

func (s *Service) SetConversationEventProjector(projector ConversationEventProjector) {
	s.projector = projector
}

func (s *Service) SetNotifier(notify func(uint)) { s.notify = notify }

func (s *Service) notifyUser(userID uint) {
	if s.notify != nil {
		s.notify(userID)
	}
}

func (s *Service) CreateArtifact(ctx context.Context, userID uint, workspaceID, fileID string) (*ArtifactView, error) {
	workspaceID, fileID = strings.TrimSpace(workspaceID), strings.TrimSpace(fileID)
	if userID == 0 || len(workspaceID) > 64 || !validOpaqueRef(workspaceID) || !validPublicID(fileID, "file") {
		return nil, ErrInvalidInput
	}
	item, err := s.repo.CreateArtifact(ctx, userID, workspaceID, fileID, &domainagent.Artifact{PublicID: newPublicID("agart")})
	if err != nil {
		return nil, mapResourceError(err)
	}
	return &ArtifactView{ArtifactID: item.PublicID, WorkspaceID: workspaceID, FileName: item.FileName, MimeType: item.MimeType, SizeBytes: item.SizeBytes, SHA256: item.SHA256}, nil
}

func (s *Service) OpenArtifact(ctx context.Context, artifactID, commandID, expires, grant string) (*ArtifactContent, error) {
	if s.artifacts == nil || !validPublicID(artifactID, "agart") || !validPublicID(commandID, "agcmd") || len(grant) != 43 {
		return nil, ErrCredential
	}
	now := s.now().UTC()
	expiresAt, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || !expiresAt.After(now) || expiresAt.After(now.Add(6*time.Minute)) {
		return nil, ErrCredential
	}
	artifact, command, err := s.repo.GetArtifactForCommand(ctx, artifactID, commandID)
	if err != nil || !sameHash(s.artifactGrant(*artifact, *command, expiresAt), grant) {
		return nil, ErrCredential
	}
	content, err := s.artifacts.OpenAgentArtifact(ctx, artifact.UserID, artifact.FileID)
	if err != nil {
		return nil, err
	}
	if content.SizeBytes != artifact.SizeBytes {
		_ = content.Reader.Close()
		return nil, ErrStateConflict
	}
	return content, nil
}

func (s *Service) BeginRuntimeProof(ctx context.Context, identity *ConnectionIdentity, profilePublicID string) (*RuntimeChallengeResult, error) {
	profilePublicID = strings.TrimSpace(profilePublicID)
	if identity == nil || identity.InternalDeviceID == 0 || len(profilePublicID) > 64 ||
		!validOpaqueRef(profilePublicID) || s.users == nil || s.proofs == nil {
		return nil, ErrRuntimeAuth
	}
	userPublicID, sub2UserID, err := s.users.RuntimeUser(ctx, identity.UserID)
	if err != nil || !validUserPublicID(userPublicID) || sub2UserID <= 0 {
		return nil, ErrRuntimeAuth
	}
	device, err := s.repo.GetDeviceByPublicID(ctx, identity.DeviceID)
	if err != nil || device.ID != identity.InternalDeviceID || device.Status != domainagent.DeviceStatusActive {
		return nil, ErrRuntimeAuth
	}
	nonceBytes := make([]byte, 32)
	if _, err = rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	profile := &domainagent.RuntimeProfile{
		PublicID: profilePublicID, UserID: identity.UserID, DeviceID: identity.InternalDeviceID,
		Provider: "codex", Status: domainagent.RuntimeStatusProving,
	}
	challenge := &domainagent.RuntimeProofChallenge{
		PublicID: newPublicID("agp"), UserID: identity.UserID, DeviceID: identity.InternalDeviceID,
		Nonce: base64.RawURLEncoding.EncodeToString(nonceBytes), ExpiresAt: now.Add(runtimeChallengeTTL),
	}
	profile, challenge, err = s.repo.BeginRuntimeProof(ctx, identity.InternalDeviceID, profilePublicID, profile, challenge, now)
	if err != nil {
		return nil, err
	}
	canonical := runtimeChallengeCanonical(
		userPublicID, identity.DeviceID, profile.PublicID, device.PublicKeyFingerprint,
		challenge.Nonce, challenge.ExpiresAt,
	)
	return &RuntimeChallengeResult{Profile: profile, Challenge: challenge, Canonical: canonical}, nil
}

func (s *Service) CompleteRuntimeProof(
	ctx context.Context,
	identity *ConnectionIdentity,
	challenge *RuntimeChallengeResult,
	proofText string,
	manifest json.RawMessage,
) (time.Time, error) {
	if identity == nil || challenge == nil || challenge.Profile == nil || challenge.Challenge == nil ||
		challenge.Profile.UserID != identity.UserID || challenge.Profile.DeviceID != identity.InternalDeviceID ||
		challenge.Challenge.DeviceID != identity.InternalDeviceID || challenge.Challenge.ExpiresAt.Before(s.now().UTC()) {
		return time.Time{}, fmt.Errorf("%w: runtime challenge validation failed", ErrRuntimeAuth)
	}
	if !validProviderManifest(manifest, challenge.Profile.Provider) {
		return time.Time{}, fmt.Errorf("%w: runtime manifest validation failed", ErrInvalidInput)
	}
	proof, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(proofText))
	if err != nil || len(proof) != sha256.Size {
		return time.Time{}, fmt.Errorf("%w: proof encoding is invalid", ErrRuntimeAuth)
	}
	_, sub2UserID, err := s.users.RuntimeUser(ctx, identity.UserID)
	if err != nil {
		return time.Time{}, fmt.Errorf("runtime user lookup: %w", err)
	}
	if sub2UserID <= 0 {
		return time.Time{}, fmt.Errorf("%w: runtime user is not linked", ErrRuntimeAuth)
	}
	remoteKeyID, credentialHash, err := s.proofs.MatchRuntimeProof(
		ctx, identity.UserID, sub2UserID, []byte(challenge.Canonical), proof,
	)
	if err != nil || remoteKeyID <= 0 || len(credentialHash) != sha256.Size*2 {
		return time.Time{}, fmt.Errorf("%w: API key ownership proof failed: %v", ErrRuntimeAuth, err)
	}
	now := s.now().UTC()
	leaseExpiresAt := now.Add(runtimeLeaseTTL)
	if err = s.repo.CompleteRuntimeProof(
		ctx, identity.InternalDeviceID, challenge.Profile.ID, challenge.Challenge.ID,
		remoteKeyID, credentialHash, string(manifest), now, leaseExpiresAt,
	); err != nil {
		return time.Time{}, fmt.Errorf("persist runtime proof: %w", err)
	}
	challenge.Profile.ManifestJSON = string(manifest)
	return leaseExpiresAt, nil
}

func (s *Service) TouchRuntimePresence(ctx context.Context, identity *ConnectionIdentity, profileID uint) error {
	if identity == nil || identity.InternalDeviceID == 0 || profileID == 0 {
		return ErrInvalidInput
	}
	now := s.now().UTC()
	return s.repo.TouchRuntimePresence(ctx, identity.InternalDeviceID, profileID, now, now.Add(runtimePresenceTTL))
}

func (s *Service) SyncWorkspaces(ctx context.Context, identity *ConnectionIdentity, challenge *RuntimeChallengeResult, registrations []WorkspaceRegistration) error {
	if identity == nil || challenge == nil || challenge.Profile == nil || len(registrations) > 128 {
		return ErrInvalidInput
	}
	items := make([]domainagent.Workspace, 0, len(registrations))
	seen := make(map[string]struct{}, len(registrations))
	for _, registration := range registrations {
		registration.WorkspaceID, registration.Name = strings.TrimSpace(registration.WorkspaceID), strings.TrimSpace(registration.Name)
		if len(registration.WorkspaceID) > 64 || !validOpaqueRef(registration.WorkspaceID) ||
			registration.Name == "" || utf8.RuneCountInString(registration.Name) > 128 {
			return ErrInvalidInput
		}
		if _, exists := seen[registration.WorkspaceID]; exists {
			return ErrInvalidInput
		}
		seen[registration.WorkspaceID] = struct{}{}
		items = append(items, domainagent.Workspace{
			PublicID: registration.WorkspaceID, UserID: identity.UserID, DeviceID: identity.InternalDeviceID,
			RuntimeProfileID: challenge.Profile.ID, Name: registration.Name, Status: "available",
		})
	}
	now := s.now().UTC()
	if err := s.repo.SyncWorkspaces(ctx, identity.UserID, identity.InternalDeviceID, challenge.Profile.ID, items, now); err != nil {
		return err
	}
	bucket := now.Truncate(time.Hour).Format(time.RFC3339)
	if runtimeProfileHasResource(challenge.Profile, "profile", "apps") {
		appsKey := uuid.NewSHA1(uuid.NameSpaceURL, []byte("deeix:apps:v1:"+identity.DeviceID+":"+challenge.Profile.PublicID+":"+bucket)).String()
		if _, err := s.QueueResourceRefresh(ctx, identity.UserID, identity.DeviceID, challenge.Profile.PublicID, "", "apps", appsKey); err != nil {
			return err
		}
	}
	for _, item := range items {
		if runtimeProfileHasResource(challenge.Profile, "workspace", "sessions") {
			key := uuid.NewSHA1(uuid.NameSpaceURL, []byte("deeix:sessions:v2:"+identity.DeviceID+":"+item.PublicID+":"+bucket)).String()
			if _, err := s.QueueResourceRefresh(ctx, identity.UserID, identity.DeviceID, "", item.PublicID, "sessions", key); err != nil {
				return err
			}
		}
		if runtimeProfileHasResource(challenge.Profile, "workspace", "skills") {
			skillsKey := uuid.NewSHA1(uuid.NameSpaceURL, []byte("deeix:skills:v1:"+identity.DeviceID+":"+item.PublicID+":"+bucket)).String()
			if _, err := s.QueueResourceRefresh(ctx, identity.UserID, identity.DeviceID, "", item.PublicID, "skills", skillsKey); err != nil {
				return err
			}
		}
	}
	return nil
}

func runtimeProfileHasResource(profile *domainagent.RuntimeProfile, scope, name string) bool {
	if profile == nil {
		return false
	}
	var manifest struct {
		Resources struct {
			Profile   []string `json:"profile"`
			Workspace []string `json:"workspace"`
		} `json:"resources"`
	}
	if json.Unmarshal([]byte(profile.ManifestJSON), &manifest) != nil {
		return false
	}
	resources := manifest.Resources.Workspace
	if scope == "profile" {
		resources = manifest.Resources.Profile
	}
	return contains(resources, name)
}

func (s *Service) ListRuntimeProfiles(ctx context.Context, userID uint, devicePublicID string) ([]RuntimeProfileView, error) {
	if userID == 0 || !validPublicID(devicePublicID, "agd") {
		return nil, ErrInvalidInput
	}
	items, err := s.repo.ListRuntimeProfiles(ctx, userID, devicePublicID)
	if err != nil {
		return nil, mapResourceError(err)
	}
	result := make([]RuntimeProfileView, 0, len(items))
	for _, item := range items {
		result = append(result, RuntimeProfileView{
			ProfileID: item.PublicID, DeviceID: devicePublicID, Provider: item.Provider,
			Status: item.Status, VerifiedAt: item.VerifiedAt, LeaseExpiresAt: item.LeaseExpiresAt,
			Manifest: json.RawMessage(item.ManifestJSON),
		})
	}
	return result, nil
}

func (s *Service) ListWorkspaces(ctx context.Context, userID uint, devicePublicID string) ([]WorkspaceView, error) {
	if userID == 0 || !validPublicID(devicePublicID, "agd") {
		return nil, ErrInvalidInput
	}
	items, err := s.repo.ListWorkspaces(ctx, userID, devicePublicID)
	if err != nil {
		return nil, mapResourceError(err)
	}
	result := make([]WorkspaceView, 0, len(items))
	for _, item := range items {
		result = append(result, WorkspaceView{
			WorkspaceID: item.PublicID, DeviceID: devicePublicID, ProfileID: item.ProfilePublicID, Name: item.Name,
			Status: item.Status, LastSeenAt: item.LastSeenAt,
		})
	}
	return result, nil
}

func (s *Service) QueueResourceRefresh(ctx context.Context, userID uint, devicePublicID, profilePublicID, workspacePublicID, resourceName, idempotencyKey string) (*ResourceRefreshView, error) {
	devicePublicID, profilePublicID = strings.TrimSpace(devicePublicID), strings.TrimSpace(profilePublicID)
	workspacePublicID, resourceName = strings.TrimSpace(workspacePublicID), strings.TrimSpace(resourceName)
	profileTarget := workspacePublicID == ""
	if userID == 0 || !validPublicID(devicePublicID, "agd") || !validIdempotencyKey(idempotencyKey) ||
		(profileTarget && (len(profilePublicID) > 64 || !validOpaqueRef(profilePublicID) || !validProfileResource(resourceName))) ||
		(!profileTarget && (profilePublicID != "" || len(workspacePublicID) > 64 || !validOpaqueRef(workspacePublicID) || !validWorkspaceResource(resourceName))) {
		return nil, ErrInvalidInput
	}
	request := struct {
		DeviceID, ProfileID, WorkspaceID, Resource string
	}{devicePublicID, profilePublicID, workspacePublicID, resourceName}
	command := &domainagent.Command{PublicID: newPublicID("agcmd"), Kind: "resource.refresh"}
	created, err := s.repo.QueueResourceRefresh(ctx, idempotencyKey, requestHash(request), userID, devicePublicID, profilePublicID, workspacePublicID, resourceName, command, s.now().UTC())
	if err != nil {
		return nil, mapResourceError(err)
	}
	s.notifyUser(userID)
	return &ResourceRefreshView{CommandID: created.PublicID, Status: created.State}, nil
}

func (s *Service) QueueAgentUpdate(ctx context.Context, userID uint, devicePublicID, idempotencyKey string) (*CommandView, error) {
	devicePublicID, idempotencyKey = strings.TrimSpace(devicePublicID), strings.TrimSpace(idempotencyKey)
	targetVersion := buildinfo.ResolveVersion()
	if userID == 0 || !validPublicID(devicePublicID, "agd") || !validIdempotencyKey(idempotencyKey) || !validAgentVersion(targetVersion) {
		return nil, ErrInvalidInput
	}
	device, err := s.repo.GetDevice(ctx, userID, devicePublicID)
	if err != nil {
		return nil, mapResourceError(err)
	}
	if !validAgentVersion(device.AgentVersion) || compareAgentVersions(device.AgentVersion, targetVersion) >= 0 {
		return nil, ErrStateConflict
	}
	request := struct {
		DeviceID, TargetVersion string
	}{devicePublicID, targetVersion}
	command := &domainagent.Command{PublicID: newPublicID("agcmd"), Kind: "agent.update"}
	created, err := s.repo.QueueAgentUpdate(
		ctx, idempotencyKey, requestHash(request), userID, devicePublicID, targetVersion, command, s.now().UTC(),
	)
	if err != nil {
		return nil, mapResourceError(err)
	}
	s.notifyUser(userID)
	return &CommandView{CommandID: created.PublicID, Status: created.State}, nil
}

func (s *Service) RegisterWorkspace(ctx context.Context, userID uint, input RegisterWorkspaceInput) (*CommandView, error) {
	input.DeviceID, input.ProfileID, input.Path = strings.TrimSpace(input.DeviceID), strings.TrimSpace(input.ProfileID), strings.TrimSpace(input.Path)
	if userID == 0 || !validPublicID(input.DeviceID, "agd") || !validOpaqueRef(input.ProfileID) ||
		!validIdempotencyKey(input.IdempotencyKey) || input.Path == "" || len(input.Path) > 4096 || strings.ContainsRune(input.Path, 0) {
		return nil, ErrInvalidInput
	}
	request := struct {
		DeviceID, ProfileID, Path string
		Create                    bool
	}{input.DeviceID, input.ProfileID, input.Path, input.Create}
	command := &domainagent.Command{PublicID: newPublicID("agcmd"), Kind: "workspace.register"}
	created, err := s.repo.QueueWorkspaceRegistration(
		ctx, input.IdempotencyKey, requestHash(request), userID,
		input.DeviceID, input.ProfileID, input.Path, input.Create, command, s.now().UTC(),
	)
	if err != nil {
		return nil, mapResourceError(err)
	}
	s.notifyUser(userID)
	return &CommandView{CommandID: created.PublicID, Status: created.State}, nil
}

func (s *Service) GetCommand(ctx context.Context, userID uint, commandID string) (*CommandView, error) {
	commandID = strings.TrimSpace(commandID)
	if userID == 0 || !validPublicID(commandID, "agcmd") {
		return nil, ErrInvalidInput
	}
	item, err := s.repo.GetCommand(ctx, userID, commandID)
	if err != nil {
		return nil, mapResourceError(err)
	}
	view := &CommandView{CommandID: item.PublicID, Status: item.State}
	if item.CompletedAt != nil {
		var outcome struct {
			Kind  string `json:"kind"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(item.TerminalJSON), &outcome) == nil && outcome.Kind == "error" {
			view.Status = "error"
			view.ErrorMessage = strings.TrimSpace(outcome.Error.Message)
		}
	}
	return view, nil
}

func (s *Service) GetResourceSnapshot(ctx context.Context, userID uint, devicePublicID, profilePublicID, workspacePublicID, resourceName string) (*ResourceSnapshotView, error) {
	devicePublicID, profilePublicID = strings.TrimSpace(devicePublicID), strings.TrimSpace(profilePublicID)
	workspacePublicID, resourceName = strings.TrimSpace(workspacePublicID), strings.TrimSpace(resourceName)
	profileTarget := workspacePublicID == ""
	if userID == 0 || !validPublicID(devicePublicID, "agd") ||
		(profileTarget && (len(profilePublicID) > 64 || !validOpaqueRef(profilePublicID) || !validProfileResource(resourceName))) ||
		(!profileTarget && (profilePublicID != "" || len(workspacePublicID) > 64 || !validOpaqueRef(workspacePublicID) || !validWorkspaceResource(resourceName))) {
		return nil, ErrInvalidInput
	}
	item, err := s.repo.GetResourceSnapshot(ctx, userID, devicePublicID, profilePublicID, workspacePublicID, resourceName)
	if err != nil {
		return nil, mapResourceError(err)
	}
	if !json.Valid([]byte(item.DataJSON)) {
		return nil, errors.New("stored agent resource snapshot is invalid")
	}
	scope := "profile"
	if workspacePublicID != "" {
		scope = "workspace"
	}
	return &ResourceSnapshotView{
		Resource: item.Name, Scope: scope, DeviceID: item.DevicePublicID,
		ProfileID: item.ProfilePublicID, WorkspaceID: item.WorkspacePublicID,
		Data: json.RawMessage(item.DataJSON), RefreshedAt: item.RefreshedAt,
	}, nil
}

func (s *Service) ResolveExecutionTarget(ctx context.Context, userID uint, deviceID, profileID, workspaceID string) (string, error) {
	deviceID, profileID, workspaceID = strings.TrimSpace(deviceID), strings.TrimSpace(profileID), strings.TrimSpace(workspaceID)
	if userID == 0 || !validPublicID(deviceID, "agd") || !validOpaqueRef(profileID) || !validOpaqueRef(workspaceID) {
		return "", ErrInvalidInput
	}
	provider, err := s.repo.ResolveExecutionTarget(ctx, userID, deviceID, profileID, workspaceID, s.now().UTC())
	return provider, mapResourceError(err)
}

func (s *Service) StartThread(ctx context.Context, userID uint, input StartThreadInput) (*StartThreadResult, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.DeviceID, input.ProfileID, input.WorkspaceID = strings.TrimSpace(input.DeviceID), strings.TrimSpace(input.ProfileID), strings.TrimSpace(input.WorkspaceID)
	if userID == 0 || input.ConversationID == 0 || !validPublicID(input.DeviceID, "agd") || len(input.ProfileID) > 64 || !validOpaqueRef(input.ProfileID) ||
		len(input.WorkspaceID) > 64 || !validOpaqueRef(input.WorkspaceID) || utf8.RuneCountInString(input.Title) > 256 ||
		!validIdempotencyKey(input.IdempotencyKey) || !validSettings(input.Settings) ||
		(len(input.InitialInput) > 0 && (!validInput(input.InitialInput) || normalizeAgentRunID(input.InitialRunID) == "")) {
		return nil, ErrInvalidInput
	}
	thread := &domainagent.Thread{PublicID: newPublicID("agth"), UserID: userID, ConversationID: input.ConversationID, Title: input.Title, Status: "queued"}
	commandPayload, _ := json.Marshal(map[string]any{
		"kind": "thread.create", "deviceId": input.DeviceID, "profileId": input.ProfileID,
		"workspaceId": input.WorkspaceID, "settings": json.RawMessage(input.Settings),
	})
	command := &domainagent.Command{PublicID: newPublicID("agcmd"), Kind: "thread.create", PayloadJSON: string(commandPayload)}
	var turn *domainagent.Turn
	if len(input.InitialInput) > 0 {
		turn = &domainagent.Turn{PublicID: newPublicID("agturn"), UserID: userID, RunID: input.InitialRunID, Status: "awaiting_thread", InputJSON: string(input.InitialInput), SettingsJSON: string(input.Settings)}
	}
	createdThread, createdTurn, err := s.repo.StartThread(ctx, input.IdempotencyKey, requestHash(input), thread, turn, command, s.now().UTC())
	if err != nil {
		return nil, mapResourceError(err)
	}
	s.notifyUser(userID)
	result := &StartThreadResult{Thread: threadView(*createdThread, input.DeviceID, input.ProfileID, input.WorkspaceID)}
	if createdTurn != nil {
		view := turnView(*createdTurn, createdThread.PublicID)
		result.Turn = &view
	}
	return result, nil
}

func (s *Service) GetThreadByConversation(ctx context.Context, userID, conversationID uint) (*ThreadView, error) {
	if userID == 0 || conversationID == 0 {
		return nil, ErrInvalidInput
	}
	item, err := s.repo.GetThreadByConversation(ctx, userID, conversationID)
	if err != nil {
		return nil, mapResourceError(err)
	}
	view := threadView(*item, item.DevicePublicID, item.ProfilePublicID, item.WorkspacePublicID)
	return &view, nil
}

func (s *Service) GetThreadHistory(ctx context.Context, userID, conversationID uint) (*ThreadHistoryView, error) {
	if userID == 0 || conversationID == 0 {
		return nil, ErrInvalidInput
	}
	item, err := s.repo.GetThreadByConversation(ctx, userID, conversationID)
	if err != nil {
		return nil, mapResourceError(err)
	}
	return &ThreadHistoryView{Status: normalizeHistoryStatus(item.HistoryStatus), Error: item.HistoryError}, nil
}

func (s *Service) EnsureThreadHistory(ctx context.Context, userID, conversationID uint) (*ThreadHistoryView, error) {
	if userID == 0 || conversationID == 0 {
		return nil, ErrInvalidInput
	}
	command := &domainagent.Command{PublicID: newPublicID("agcmd"), Kind: "thread.read"}
	thread, queued, err := s.repo.QueueThreadHistory(ctx, userID, conversationID, command, s.now().UTC())
	if err != nil {
		return nil, mapResourceError(err)
	}
	if queued != nil {
		s.notifyUser(userID)
	}
	return &ThreadHistoryView{Status: normalizeHistoryStatus(thread.HistoryStatus), Error: thread.HistoryError}, nil
}

func (s *Service) DeleteThread(ctx context.Context, userID uint, threadID, idempotencyKey string) (*CommandView, error) {
	threadID = strings.TrimSpace(threadID)
	if userID == 0 || !validPublicID(threadID, "agth") || !validIdempotencyKey(idempotencyKey) {
		return nil, ErrInvalidInput
	}
	command := &domainagent.Command{PublicID: newPublicID("agcmd"), Kind: "thread.lifecycle"}
	request := struct {
		ThreadID string
		Action   string
	}{ThreadID: threadID, Action: "delete"}
	created, err := s.repo.QueueThreadLifecycle(ctx, idempotencyKey, requestHash(request), userID, threadID, "delete", command, s.now().UTC())
	if err != nil {
		return nil, mapResourceError(err)
	}
	s.notifyUser(userID)
	return &CommandView{CommandID: created.PublicID, Status: created.State}, nil
}

func (s *Service) SetThreadArchived(ctx context.Context, userID uint, threadID string, archived bool, idempotencyKey string) (*CommandView, error) {
	threadID = strings.TrimSpace(threadID)
	if userID == 0 || !validPublicID(threadID, "agth") || !validIdempotencyKey(idempotencyKey) {
		return nil, ErrInvalidInput
	}
	action := "unarchive"
	if archived {
		action = "archive"
	}
	command := &domainagent.Command{PublicID: newPublicID("agcmd"), Kind: "thread.lifecycle"}
	request := struct {
		ThreadID string
		Action   string
	}{ThreadID: threadID, Action: action}
	created, err := s.repo.QueueThreadLifecycle(ctx, idempotencyKey, requestHash(request), userID, threadID, action, command, s.now().UTC())
	if err != nil {
		return nil, mapResourceError(err)
	}
	s.notifyUser(userID)
	return &CommandView{CommandID: created.PublicID, Status: created.State}, nil
}

func (s *Service) StartTurn(ctx context.Context, userID uint, input StartTurnInput) (*TurnView, error) {
	if userID == 0 || normalizeAgentRunID(input.RunID) == "" || !validPublicID(input.ThreadID, "agth") || !validIdempotencyKey(input.IdempotencyKey) ||
		!validInput(input.Input) || !validSettings(input.Settings) {
		return nil, ErrInvalidInput
	}
	turn := &domainagent.Turn{PublicID: newPublicID("agturn"), UserID: userID, ThreadPublicID: input.ThreadID, RunID: input.RunID, Status: "queued", InputJSON: string(input.Input), SettingsJSON: string(input.Settings)}
	command := &domainagent.Command{PublicID: newPublicID("agcmd"), Kind: "turn.start"}
	created, err := s.repo.StartTurn(ctx, input.IdempotencyKey, requestHash(input), turn, command, s.now().UTC())
	if err != nil {
		return nil, mapResourceError(err)
	}
	s.notifyUser(userID)
	view := turnView(*created, input.ThreadID)
	return &view, nil
}

func (s *Service) InterruptRun(ctx context.Context, userID uint, runID, idempotencyKey string) (*CommandView, error) {
	if userID == 0 || normalizeAgentRunID(runID) == "" || !validIdempotencyKey(idempotencyKey) {
		return nil, ErrInvalidInput
	}
	turn, err := s.repo.GetTurnByRunID(ctx, userID, runID)
	if err != nil {
		return nil, mapResourceError(err)
	}
	command := &domainagent.Command{PublicID: newPublicID("agcmd"), Kind: "turn.interrupt"}
	request := struct{ RunID string }{RunID: runID}
	created, err := s.repo.QueueTurnInterrupt(ctx, idempotencyKey, requestHash(request), userID, turn.PublicID, command, s.now().UTC())
	if err != nil {
		return nil, mapResourceError(err)
	}
	s.notifyUser(userID)
	return &CommandView{CommandID: created.PublicID, Status: created.State}, nil
}

func (s *Service) ListInteractions(ctx context.Context, userID uint, threadPublicID, status string) ([]InteractionView, error) {
	status = strings.TrimSpace(status)
	if !validPublicID(threadPublicID, "agth") || (status != "" && !contains([]string{"pending", "responding", "resolved", "failed"}, status)) {
		return nil, ErrInvalidInput
	}
	items, err := s.repo.ListInteractions(ctx, userID, threadPublicID, status, 100)
	if err != nil {
		return nil, mapResourceError(err)
	}
	result := make([]InteractionView, 0, len(items))
	for _, item := range items {
		result = append(result, InteractionView{InteractionID: item.PublicID, ThreadID: threadPublicID, TurnID: item.TurnPublicID, RunID: item.RunID, Kind: item.Kind, Status: item.Status, Request: json.RawMessage(item.RequestJSON), CreatedAt: item.CreatedAt})
	}
	return result, nil
}

func (s *Service) RespondInteraction(ctx context.Context, userID uint, input RespondInteractionInput) (*InteractionView, error) {
	if userID == 0 || !validPublicID(input.InteractionID, "agint") || !validIdempotencyKey(input.IdempotencyKey) || !validInteractionResponse(input.Response) {
		return nil, ErrInvalidInput
	}
	command := &domainagent.Command{PublicID: newPublicID("agcmd"), Kind: "interaction.respond"}
	item, err := s.repo.RespondInteraction(ctx, input.IdempotencyKey, requestHash(input), userID, input.InteractionID, input.Response, command, s.now().UTC())
	if err != nil {
		return nil, mapResourceError(err)
	}
	s.notifyUser(userID)
	view := InteractionView{InteractionID: item.PublicID, ThreadID: item.ThreadPublicID, TurnID: item.TurnPublicID, RunID: item.RunID, Kind: item.Kind, Status: item.Status, Request: json.RawMessage(item.RequestJSON), CreatedAt: item.CreatedAt}
	return &view, nil
}

func mapResourceError(err error) error {
	switch {
	case errors.Is(err, repository.ErrInvalidInput):
		return ErrInvalidInput
	case errors.Is(err, repository.ErrNotFound):
		return ErrResourceNotFound
	case errors.Is(err, repository.ErrConflict):
		return ErrStateConflict
	default:
		return err
	}
}

func runtimeChallengeCanonical(userPublicID, devicePublicID, profilePublicID, fingerprint, nonce string, expiresAt time.Time) string {
	return strings.Join([]string{
		"deeix-runtime-auth-proof-v1", userPublicID, devicePublicID, profilePublicID,
		fingerprint, nonce, fmt.Sprintf("%d", expiresAt.Unix()),
	}, "\n")
}

func enrollmentChallengeCanonical(userPublicID, fingerprint, platform, nonce string, expiresAt time.Time) string {
	return strings.Join([]string{
		"deeix-device-enrollment-v1", userPublicID, fingerprint, platform,
		nonce, fmt.Sprintf("%d", expiresAt.Unix()),
	}, "\n")
}

func (s *Service) BeginEnrollment(ctx context.Context, input BeginEnrollmentInput) (*EnrollmentChallengeResult, error) {
	userPublicID := strings.TrimSpace(input.UserPublicID)
	name := strings.TrimSpace(input.Name)
	platform := strings.ToLower(strings.TrimSpace(input.Platform))
	publicKey, err := decodePublicKey(input.PublicKey)
	if err != nil || !validUserPublicID(userPublicID) || !validDeviceName(name) || !validPlatform(platform) ||
		s.users == nil || s.proofs == nil {
		return nil, ErrInvalidInput
	}
	userID, canonicalUserPublicID, remoteUserID, err := s.users.RuntimeUserByPublicID(ctx, userPublicID)
	if err != nil || userID == 0 || remoteUserID <= 0 || canonicalUserPublicID != userPublicID {
		return nil, ErrRuntimeAuth
	}
	nonceBytes := make([]byte, 32)
	if _, err = rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	fingerprint := sha256.Sum256(publicKey)
	now := s.now().UTC()
	item := domainagent.DeviceEnrollmentChallenge{
		PublicID: newPublicID("age"), UserID: userID, UserPublicID: userPublicID,
		RemoteUserID: remoteUserID, Name: name, Platform: platform, PublicKey: publicKey,
		PublicKeyFingerprint: hex.EncodeToString(fingerprint[:]),
		Nonce:                base64.RawURLEncoding.EncodeToString(nonceBytes), ExpiresAt: now.Add(enrollmentTTL),
	}
	if err := s.repo.CreateEnrollmentChallenge(ctx, &item); err != nil {
		return nil, err
	}
	return &EnrollmentChallengeResult{
		ChallengeID: item.PublicID,
		Canonical:   enrollmentChallengeCanonical(userPublicID, item.PublicKeyFingerprint, platform, item.Nonce, item.ExpiresAt),
		ExpiresAt:   item.ExpiresAt,
	}, nil
}

func (s *Service) CompleteEnrollment(ctx context.Context, input CompleteEnrollmentInput) (*EnrollDeviceResult, error) {
	challengeID := strings.TrimSpace(input.ChallengeID)
	proof, proofErr := base64.RawURLEncoding.DecodeString(strings.TrimSpace(input.Proof))
	signature, signatureErr := base64.RawURLEncoding.DecodeString(strings.TrimSpace(input.Signature))
	if !validPublicID(challengeID, "age") || proofErr != nil || len(proof) != sha256.Size ||
		signatureErr != nil || len(signature) != ed25519.SignatureSize || s.proofs == nil {
		return nil, ErrInvalidInput
	}
	challenge, err := s.repo.GetEnrollmentChallenge(ctx, challengeID)
	if err != nil || challenge.ExpiresAt.Before(s.now().UTC()) {
		return nil, ErrCredential
	}
	canonical := enrollmentChallengeCanonical(
		challenge.UserPublicID, challenge.PublicKeyFingerprint, challenge.Platform,
		challenge.Nonce, challenge.ExpiresAt,
	)
	if !ed25519.Verify(challenge.PublicKey, []byte(canonical), signature) {
		return nil, ErrInvalidSignature
	}
	if _, _, err = s.proofs.MatchRuntimeProof(ctx, challenge.UserID, challenge.RemoteUserID, []byte(canonical), proof); err != nil {
		return nil, ErrRuntimeAuth
	}
	item := domainagent.Device{
		PublicID: newPublicID("agd"), UserID: challenge.UserID, Name: challenge.Name, Platform: challenge.Platform,
		PublicKey: challenge.PublicKey, PublicKeyFingerprint: challenge.PublicKeyFingerprint,
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive, NextServerSeq: 1,
	}
	created, err := s.repo.ConsumeEnrollmentChallengeAndEnroll(ctx, challenge.ID, &item, s.now().UTC())
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrConflict) {
			return nil, ErrCredential
		}
		return nil, err
	}
	return &EnrollDeviceResult{DeviceID: created.PublicID, Status: created.Status}, nil
}

func (s *Service) ListDevices(ctx context.Context, userID uint, userPublicID string) ([]DeviceView, error) {
	if userID == 0 || !validUserPublicID(userPublicID) {
		return nil, ErrInvalidInput
	}
	items, err := s.repo.ListDevices(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]DeviceView, 0, len(items))
	for i := range items {
		result = append(result, deviceView(items[i], userPublicID))
	}
	return result, nil
}

func (s *Service) GetDevice(ctx context.Context, userID uint, userPublicID, devicePublicID string) (*DeviceView, error) {
	if userID == 0 || !validUserPublicID(userPublicID) || !validPublicID(devicePublicID, "agd") {
		return nil, ErrInvalidInput
	}
	item, err := s.repo.GetDevice(ctx, userID, devicePublicID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}
	view := deviceView(*item, userPublicID)
	return &view, nil
}

func (s *Service) RenameDevice(ctx context.Context, userID uint, userPublicID, devicePublicID, name string) (*DeviceView, error) {
	name = strings.TrimSpace(name)
	if userID == 0 || !validUserPublicID(userPublicID) || !validPublicID(devicePublicID, "agd") || !validDeviceName(name) {
		return nil, ErrInvalidInput
	}
	item, err := s.repo.RenameDevice(ctx, userID, devicePublicID, name)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}
	view := deviceView(*item, userPublicID)
	return &view, nil
}

func (s *Service) RevokeDevice(ctx context.Context, userID uint, devicePublicID string) error {
	if userID == 0 || !validPublicID(devicePublicID, "agd") {
		return ErrInvalidInput
	}
	if err := s.repo.RevokeDevice(ctx, userID, devicePublicID, s.now().UTC()); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrDeviceNotFound
		}
		return err
	}
	s.notifyUser(userID)
	return nil
}

func (s *Service) CreateChallenge(ctx context.Context, devicePublicID string) (*ChallengeResult, error) {
	if !validPublicID(devicePublicID, "agd") {
		return nil, ErrInvalidInput
	}
	device, err := s.repo.GetDeviceByPublicID(ctx, devicePublicID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}
	if device.Status != domainagent.DeviceStatusActive {
		return nil, ErrDeviceRevoked
	}
	now := s.now().UTC()
	item := domainagent.Credential{
		PublicID: newPublicID("agc"), UserID: device.UserID, DeviceID: &device.ID,
		Kind: domainagent.CredentialKindChallenge, DerivationKeyVersion: credentialKeyVersion,
		DeviceCredentialVersion: device.CredentialVersion, ExpiresAt: now.Add(challengeTTL),
	}
	bearer := s.deriveBearer(&item)
	item.TokenHash = hashBearer(bearer)
	if err = s.repo.CreateDeviceCredential(ctx, device.ID, &item); err != nil {
		if errors.Is(err, repository.ErrConflict) {
			return nil, ErrDeviceRevoked
		}
		return nil, err
	}
	return &ChallengeResult{ChallengeID: item.PublicID, Challenge: bearer, ExpiresAt: item.ExpiresAt}, nil
}

func (s *Service) IssueConnection(ctx context.Context, devicePublicID, challengePublicID, signatureText string) (*ConnectionResult, error) {
	if !validPublicID(devicePublicID, "agd") || !validPublicID(challengePublicID, "agc") {
		return nil, ErrInvalidInput
	}
	device, err := s.repo.GetDeviceByPublicID(ctx, devicePublicID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}
	if device.Status != domainagent.DeviceStatusActive {
		return nil, ErrDeviceRevoked
	}
	challenge, err := s.repo.GetCredential(ctx, challengePublicID, domainagent.CredentialKindChallenge)
	if err != nil || challenge.DeviceID == nil || *challenge.DeviceID != device.ID ||
		challenge.DeviceCredentialVersion != device.CredentialVersion || challenge.ExpiresAt.Before(s.now().UTC()) {
		return nil, ErrCredential
	}
	challengeBearer := s.deriveBearer(challenge)
	if !sameHash(hashBearer(challengeBearer), challenge.TokenHash) {
		return nil, ErrCredential
	}
	signature, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(signatureText))
	if err != nil || len(signature) != ed25519.SignatureSize ||
		!ed25519.Verify(ed25519.PublicKey(device.PublicKey), []byte(challengeBearer), signature) {
		return nil, ErrInvalidSignature
	}
	now := s.now().UTC()
	connection := domainagent.Credential{
		PublicID: newPublicID("agc"), UserID: device.UserID, DeviceID: &device.ID,
		ParentCredentialID: &challenge.ID, Kind: domainagent.CredentialKindConnection,
		DerivationKeyVersion: credentialKeyVersion, DeviceCredentialVersion: device.CredentialVersion,
		ExpiresAt: now.Add(connectionTTL),
	}
	connectionBearer := s.deriveBearer(&connection)
	connection.TokenHash = hashBearer(connectionBearer)
	created, err := s.repo.ConsumeChallengeAndCreateConnection(ctx, device.ID, challenge.ID, &connection, now)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrConflict) {
			return nil, ErrCredential
		}
		return nil, err
	}
	connectionBearer = s.deriveBearer(created)
	if !sameHash(hashBearer(connectionBearer), created.TokenHash) {
		return nil, ErrCredential
	}
	return &ConnectionResult{ConnectionToken: connectionBearer, ExpiresAt: created.ExpiresAt}, nil
}

func (s *Service) AuthenticateConnection(ctx context.Context, token string) (*ConnectionIdentity, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, credentialPrefix(domainagent.CredentialKindConnection)) || len(token) > 128 {
		return nil, ErrCredential
	}
	device, err := s.repo.ConsumeConnection(ctx, hashBearer(token), s.now().UTC())
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrConflict) {
			return nil, ErrCredential
		}
		return nil, err
	}
	return &ConnectionIdentity{
		InternalDeviceID: device.ID, DeviceID: device.PublicID, UserID: device.UserID,
		LastAckedServerSeq: device.LastAckedServerSeq, LastAckedBridgeSeq: device.LastAckedBridgeSeq,
	}, nil
}

func (s *Service) CommandsForDelivery(ctx context.Context, identity *ConnectionIdentity, after uint64) ([]DeliveryCommand, error) {
	if identity == nil || identity.InternalDeviceID == 0 || after > uint64(^uint(0)>>1) {
		return nil, ErrInvalidInput
	}
	items, err := s.repo.ListCommandsForDelivery(ctx, identity.InternalDeviceID, after, 1)
	if err != nil {
		return nil, err
	}
	result := make([]DeliveryCommand, 0, len(items))
	for _, item := range items {
		if !json.Valid([]byte(item.PayloadJSON)) {
			return nil, errors.New("stored agent command is invalid")
		}
		refs, err := commandArtifactRefs(item.PayloadJSON)
		if err != nil {
			return nil, errors.New("stored agent command artifact input is invalid")
		}
		artifacts, err := s.repo.ListArtifactsForCommand(ctx, identity.InternalDeviceID, item.ID, refs)
		if err != nil {
			return nil, err
		}
		expiresAt := s.now().UTC().Add(5 * time.Minute)
		grants := make([]ArtifactGrant, 0, len(artifacts))
		for _, artifact := range artifacts {
			grants = append(grants, ArtifactGrant{
				ArtifactRef: artifact.PublicID, FileName: artifact.FileName,
				MimeType: artifact.MimeType, SizeBytes: artifact.SizeBytes, SHA256: artifact.SHA256,
				ExpiresAt: expiresAt.Format(time.RFC3339Nano), Grant: s.artifactGrant(artifact, item, expiresAt),
			})
		}
		result = append(result, DeliveryCommand{InternalID: item.ID, CommandID: item.PublicID, ServerSeq: item.ServerSeq, Command: json.RawMessage(item.PayloadJSON), Artifacts: grants})
	}
	return result, nil
}

func commandArtifactRefs(payload string) ([]string, error) {
	var command struct {
		Kind  string `json:"kind"`
		Input []struct {
			Kind        string `json:"kind"`
			ArtifactRef string `json:"artifactRef"`
		} `json:"input"`
	}
	if err := json.Unmarshal([]byte(payload), &command); err != nil {
		return nil, err
	}
	if command.Kind != "turn.start" && command.Kind != "turn.steer" {
		return []string{}, nil
	}
	seen := make(map[string]struct{})
	refs := make([]string, 0)
	for _, input := range command.Input {
		if input.Kind != "artifact" {
			continue
		}
		if !validPublicID(input.ArtifactRef, "agart") {
			return nil, ErrInvalidInput
		}
		if _, ok := seen[input.ArtifactRef]; ok {
			continue
		}
		seen[input.ArtifactRef] = struct{}{}
		refs = append(refs, input.ArtifactRef)
	}
	return refs, nil
}

func (s *Service) artifactGrant(artifact domainagent.Artifact, command domainagent.Command, expiresAt time.Time) string {
	payload := fmt.Sprintf("deeix-agent-artifact-v1\n%s\n%s\n%d\n%d\n%d\n%d",
		artifact.PublicID, command.PublicID, artifact.UserID, command.DeviceID, artifact.WorkspaceID, expiresAt.Unix())
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Service) MarkCommandDelivered(ctx context.Context, identity *ConnectionIdentity, commandInternalID uint) error {
	if identity == nil || identity.InternalDeviceID == 0 || commandInternalID == 0 {
		return ErrInvalidInput
	}
	return s.repo.MarkCommandDelivered(ctx, identity.InternalDeviceID, commandInternalID, s.now().UTC())
}

func (s *Service) AckServerCommands(ctx context.Context, identity *ConnectionIdentity, through uint64) error {
	if identity == nil || identity.InternalDeviceID == 0 {
		return ErrInvalidInput
	}
	if err := s.repo.AckServerCommands(ctx, identity.InternalDeviceID, through, s.now().UTC()); err != nil {
		if errors.Is(err, repository.ErrConflict) {
			return ErrCredential
		}
		return err
	}
	return nil
}

func (s *Service) ApplyTerminalFrame(ctx context.Context, identity *ConnectionIdentity, bridgeSeq, serverSeq uint64, commandID string, outcome json.RawMessage) (uint64, error) {
	if identity == nil || identity.InternalDeviceID == 0 || bridgeSeq == 0 || serverSeq == 0 ||
		!validPublicID(commandID, "agcmd") || len(outcome) == 0 || len(outcome) > 2*1024*1024 || !validTerminalOutcome(outcome) {
		return 0, ErrInvalidInput
	}
	payloadHash := sha256.Sum256(outcome)
	acknowledged, err := s.repo.ApplyTerminalFrame(
		ctx, identity.InternalDeviceID, bridgeSeq, serverSeq, commandID,
		hex.EncodeToString(payloadHash[:]), string(outcome), s.now().UTC(),
	)
	if err != nil {
		if errors.Is(err, repository.ErrConflict) || errors.Is(err, repository.ErrNotFound) {
			return 0, ErrCredential
		}
		return 0, err
	}
	if err := s.flushPendingConversationEvents(ctx, identity.InternalDeviceID); err != nil {
		return 0, err
	}
	s.notifyUser(identity.UserID)
	return acknowledged, nil
}

func (s *Service) ApplyEventFrame(ctx context.Context, identity *ConnectionIdentity, runtimeProfileID uint, bridgeSeq uint64, event json.RawMessage) (uint64, error) {
	if identity == nil || identity.InternalDeviceID == 0 || runtimeProfileID == 0 || bridgeSeq == 0 ||
		len(event) == 0 || len(event) > 2*1024*1024 || !validProviderEvent(event) {
		return 0, ErrInvalidInput
	}
	var envelope struct {
		Kind             string          `json:"kind"`
		SourceThreadRef  string          `json:"sourceThreadRef"`
		SourceTurnRef    string          `json:"sourceTurnRef"`
		SourceItemRef    string          `json:"sourceItemRef"`
		SourceRequestRef string          `json:"sourceRequestRef"`
		OccurredAt       time.Time       `json:"occurredAt"`
		Payload          json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(event, &envelope) != nil || !json.Valid(envelope.Payload) {
		return 0, ErrInvalidInput
	}
	payloadHash := sha256.Sum256(event)
	applied, err := s.repo.ApplyEventFrame(
		ctx, identity.InternalDeviceID, runtimeProfileID, bridgeSeq,
		hex.EncodeToString(payloadHash[:]), &domainagent.Event{
			PublicID: newPublicID("agev"), UserID: identity.UserID, DeviceID: identity.InternalDeviceID,
			RuntimeProfileID: &runtimeProfileID, Kind: envelope.Kind, SourceThreadRef: envelope.SourceThreadRef,
			SourceTurnRef: envelope.SourceTurnRef, SourceItemRef: envelope.SourceItemRef,
			SourceRequestRef: envelope.SourceRequestRef, PayloadJSON: string(envelope.Payload), OccurredAt: envelope.OccurredAt,
		}, s.now().UTC(),
	)
	if err != nil {
		if errors.Is(err, repository.ErrConflict) || errors.Is(err, repository.ErrNotFound) {
			return 0, ErrCredential
		}
		return 0, err
	}
	if applied.ConversationID != 0 && applied.RunID != "" {
		if err := s.projectConversationEvent(ctx, *applied); err != nil {
			return 0, err
		}
	}
	s.notifyUser(identity.UserID)
	return applied.Acknowledged, nil
}

func (s *Service) FlushPendingConversationEvents(ctx context.Context, identity *ConnectionIdentity) error {
	if identity == nil || identity.InternalDeviceID == 0 {
		return ErrInvalidInput
	}
	return s.flushPendingConversationEvents(ctx, identity.InternalDeviceID)
}

func (s *Service) flushPendingConversationEvents(ctx context.Context, deviceID uint) error {
	for {
		items, err := s.repo.ListPendingConversationEvents(ctx, deviceID, 1000)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := s.projectConversationEvent(ctx, item); err != nil {
				return err
			}
		}
		if len(items) < 1000 {
			return nil
		}
	}
}

func (s *Service) projectConversationEvent(ctx context.Context, item domainagent.AppliedEventFrame) error {
	if s.projector == nil {
		return ErrStateConflict
	}
	if err := s.projector(ctx, item); err != nil {
		return err
	}
	return s.repo.MarkConversationEventProjected(ctx, item.Event.ID, s.now().UTC())
}

func validProviderEvent(value json.RawMessage) bool {
	var event map[string]json.RawMessage
	if json.Unmarshal(value, &event) != nil || len(event) < 3 || len(event) > 8 {
		return false
	}
	allowed := map[string]bool{
		"kind": true, "sourceThreadRef": true, "sourceTurnRef": true,
		"sourceItemRef": true, "sourceRequestRef": true, "occurredAt": true, "payload": true,
	}
	for key := range event {
		if !allowed[key] {
			return false
		}
	}
	var kind, occurredAt string
	var payload map[string]any
	if json.Unmarshal(event["kind"], &kind) != nil || kind == "" || len(kind) > 256 ||
		json.Unmarshal(event["occurredAt"], &occurredAt) != nil ||
		json.Unmarshal(event["payload"], &payload) != nil {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, occurredAt); err != nil {
		return false
	}
	for _, field := range []string{"sourceThreadRef", "sourceTurnRef", "sourceItemRef", "sourceRequestRef"} {
		if raw := event[field]; raw != nil {
			var ref string
			if json.Unmarshal(raw, &ref) != nil || !validOpaqueRef(ref) {
				return false
			}
		}
	}
	return true
}

func validOpaqueRef(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || (index > 0 && strings.ContainsRune("._:-", character)) {
			continue
		}
		return false
	}
	return true
}

func validIdempotencyKey(value string) bool {
	value = strings.TrimSpace(value)
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == strings.ToLower(value)
}

func normalizeAgentRunID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < len("run_")+1 || len(value) > 64 || !strings.HasPrefix(value, "run_") {
		return ""
	}
	for _, character := range value[len("run_"):] {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '_' || character == '-' {
			continue
		}
		return ""
	}
	return value
}

func validSettings(value json.RawMessage) bool {
	if len(value) == 0 || len(value) > 16*1024 {
		return false
	}
	var settings map[string]json.RawMessage
	if json.Unmarshal(value, &settings) != nil || len(settings) > 4 {
		return false
	}
	allowed := map[string][]string{
		"reasoningEffort": {"low", "medium", "high", "xhigh"},
		"approvalPolicy":  {"untrusted", "on-request", "never"},
		"sandboxPolicy":   {"read-only", "workspace-write"},
	}
	for key, raw := range settings {
		if key == "model" {
			var model string
			if json.Unmarshal(raw, &model) != nil || strings.TrimSpace(model) == "" || len(model) > 256 {
				return false
			}
			continue
		}
		values, ok := allowed[key]
		if !ok {
			return false
		}
		var value string
		if json.Unmarshal(raw, &value) != nil || !contains(values, value) {
			return false
		}
	}
	return true
}

func validInput(value json.RawMessage) bool {
	if len(value) == 0 || len(value) > 1024*1024+(64*1024) {
		return false
	}
	var items []map[string]json.RawMessage
	if json.Unmarshal(value, &items) != nil || len(items) == 0 || len(items) > 64 {
		return false
	}
	textBytes := 0
	for _, item := range items {
		var kind string
		if json.Unmarshal(item["kind"], &kind) != nil {
			return false
		}
		switch kind {
		case "text":
			if len(item) != 2 {
				return false
			}
			var text string
			if json.Unmarshal(item["text"], &text) != nil || text == "" || !utf8.ValidString(text) {
				return false
			}
			textBytes += len([]byte(text))
		case "artifact":
			if len(item) != 2 {
				return false
			}
			var artifactRef string
			if json.Unmarshal(item["artifactRef"], &artifactRef) != nil || !validPublicID(artifactRef, "agart") {
				return false
			}
		case "skill", "app-mention":
			if len(item) != 2 {
				return false
			}
			var resourceRef string
			if json.Unmarshal(item["resourceRef"], &resourceRef) != nil || !validOpaqueRef(resourceRef) || len(resourceRef) > 256 {
				return false
			}
		default:
			return false
		}
	}
	return textBytes <= 1024*1024
}

func validInteractionResponse(value json.RawMessage) bool {
	if len(value) == 0 || len(value) > 1024*1024 {
		return false
	}
	var response map[string]json.RawMessage
	if json.Unmarshal(value, &response) != nil {
		return false
	}
	var kind string
	if json.Unmarshal(response["kind"], &kind) != nil {
		return false
	}
	switch kind {
	case "approval":
		return len(response) == 2 && validDecision(response["decision"])
	case "user-input":
		return len(response) == 2 && validStringRecord(response["answers"])
	case "permission":
		if len(response) < 2 || len(response) > 3 || !validDecision(response["decision"]) {
			return false
		}
		if response["scope"] == nil {
			return true
		}
		var scope string
		return json.Unmarshal(response["scope"], &scope) == nil && (scope == "turn" || scope == "session")
	case "mcp-elicitation":
		if len(response) < 2 || len(response) > 3 || !validDecision(response["decision"]) {
			return false
		}
		var decision string
		_ = json.Unmarshal(response["decision"], &decision)
		return response["content"] == nil || (decision == "accept" && validStringRecord(response["content"]))
	case "dynamic-tool":
		if len(response) != 3 {
			return false
		}
		var success bool
		var content []map[string]json.RawMessage
		if json.Unmarshal(response["success"], &success) != nil || json.Unmarshal(response["content"], &content) != nil || len(content) > 64 {
			return false
		}
		for _, item := range content {
			var itemKind, text string
			if json.Unmarshal(item["kind"], &itemKind) != nil || len(item) != 2 {
				return false
			}
			field := "url"
			if itemKind == "text" {
				field = "text"
			} else if itemKind != "image" && itemKind != "audio" {
				return false
			}
			if json.Unmarshal(item[field], &text) != nil || text == "" || len(text) > 1024*1024 {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func validProviderManifest(value json.RawMessage, provider string) bool {
	if len(value) == 0 || len(value) > 64*1024 {
		return false
	}
	var manifest struct {
		AgentVersion    string   `json:"agentVersion"`
		Provider        string   `json:"provider"`
		RuntimeVersion  string   `json:"runtimeVersion"`
		ProtocolVersion string   `json:"protocolVersion"`
		SchemaHash      string   `json:"schemaHash"`
		Commands        []string `json:"commands"`
		Resources       struct {
			Profile   []string `json:"profile"`
			Workspace []string `json:"workspace"`
		} `json:"resources"`
		InputKinds     []string `json:"inputKinds"`
		ThreadSettings struct {
			Model           *bool    `json:"model"`
			ReasoningEffort []string `json:"reasoningEffort"`
			ApprovalPolicy  []string `json:"approvalPolicy"`
			SandboxPolicy   []string `json:"sandboxPolicy"`
		} `json:"threadSettings"`
		InteractionKinds []string `json:"interactionKinds"`
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&manifest) != nil || decoder.Decode(&struct{}{}) != io.EOF ||
		manifest.Provider != provider || manifest.ThreadSettings.Model == nil ||
		(manifest.AgentVersion != "" && !validAgentVersion(manifest.AgentVersion)) ||
		!validManifestText(manifest.RuntimeVersion, 64) ||
		!validManifestText(manifest.ProtocolVersion, 64) || len(manifest.SchemaHash) != 64 {
		return false
	}
	if _, err := hex.DecodeString(manifest.SchemaHash); err != nil {
		return false
	}
	return validManifestValues(manifest.Commands, []string{
		"agent.update", "workspace.register", "thread.create", "thread.lifecycle", "thread.rename", "thread.metadata.update", "thread.compact",
		"thread.read", "review.start", "turn.start", "turn.steer", "turn.interrupt", "interaction.respond", "resource.refresh",
	}) && validManifestValues(manifest.Resources.Profile, []string{
		"models", "model-capabilities", "permission-profiles", "apps", "mcp", "plugins", "auth-status",
	}) && validManifestValues(manifest.Resources.Workspace, []string{"sessions", "skills", "hooks"}) &&
		validManifestValues(manifest.InputKinds, []string{"text", "artifact", "skill", "app-mention"}) &&
		validManifestValues(manifest.ThreadSettings.ReasoningEffort, []string{"low", "medium", "high", "xhigh"}) &&
		validManifestValues(manifest.ThreadSettings.ApprovalPolicy, []string{"untrusted", "on-request", "never"}) &&
		validManifestValues(manifest.ThreadSettings.SandboxPolicy, []string{"read-only", "workspace-write"}) &&
		validManifestValues(manifest.InteractionKinds, []string{
			"command_approval", "file_approval", "user_input", "permission", "mcp_elicitation", "dynamic_tool",
		})
}

func validManifestValues(values, allowed []string) bool {
	if len(values) == 0 || len(values) > len(allowed) {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !contains(allowed, value) {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validManifestText(value string, limit int) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= limit && utf8.ValidString(value)
}

func validDecision(raw json.RawMessage) bool {
	var value string
	return json.Unmarshal(raw, &value) == nil && (value == "accept" || value == "decline")
}

func validStringRecord(raw json.RawMessage) bool {
	var value map[string]string
	if json.Unmarshal(raw, &value) != nil || len(value) > 128 {
		return false
	}
	for key, item := range value {
		if !validOpaqueRef(key) || len(item) > 64*1024 {
			return false
		}
	}
	return true
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func requestHash(value any) string {
	encoded, _ := json.Marshal(value)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

func threadView(item domainagent.Thread, deviceID, profileID, workspaceID string) ThreadView {
	return ThreadView{ThreadID: item.PublicID, DeviceID: deviceID, ProfileID: profileID, WorkspaceID: workspaceID, Title: item.Title, Status: item.Status, HistoryStatus: normalizeHistoryStatus(item.HistoryStatus), HistoryError: item.HistoryError, GitSHA: item.GitSHA, GitBranch: item.GitBranch, GitOriginURL: item.GitOriginURL, LastEventSeq: item.LastEventSeq, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func normalizeHistoryStatus(value string) string {
	if value == "unloaded" || value == "loading" || value == "error" {
		return value
	}
	return "loaded"
}

func turnView(item domainagent.Turn, threadID string) TurnView {
	return TurnView{TurnID: item.PublicID, ThreadID: threadID, Status: item.Status, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func validTerminalOutcome(value json.RawMessage) bool {
	var outcome map[string]json.RawMessage
	if json.Unmarshal(value, &outcome) != nil {
		return false
	}
	var kind string
	if json.Unmarshal(outcome["kind"], &kind) != nil {
		return false
	}
	switch kind {
	case "result":
		if len(outcome) != 2 || outcome["result"] == nil {
			return false
		}
		var result map[string]any
		return json.Unmarshal(outcome["result"], &result) == nil
	case "error":
		if len(outcome) != 2 || outcome["error"] == nil {
			return false
		}
		var errorValue map[string]json.RawMessage
		if json.Unmarshal(outcome["error"], &errorValue) != nil || len(errorValue) != 2 {
			return false
		}
		var code, message string
		return json.Unmarshal(errorValue["code"], &code) == nil &&
			json.Unmarshal(errorValue["message"], &message) == nil &&
			code != "" && len(code) <= 128 && message != "" && len(message) <= 4096
	default:
		return false
	}
}

func (s *Service) deriveBearer(item *domainagent.Credential) string {
	deviceID := uint(0)
	if item.DeviceID != nil {
		deviceID = *item.DeviceID
	}
	parentID := uint(0)
	if item.ParentCredentialID != nil {
		parentID = *item.ParentCredentialID
	}
	payload := fmt.Sprintf("deeix-agent-credential-v1\n%d\n%s\n%s\n%d\n%d\n%d\n%d\n%d",
		item.DerivationKeyVersion, item.Kind, item.PublicID, item.UserID, deviceID,
		parentID, item.DeviceCredentialVersion, item.ExpiresAt.Unix())
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(payload))
	return credentialPrefix(item.Kind) + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func credentialPrefix(kind string) string {
	switch kind {
	case domainagent.CredentialKindChallenge:
		return "deeix_challenge_"
	default:
		return "deeix_connection_"
	}
}

func hashBearer(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func sameHash(left, right string) bool {
	return hmac.Equal([]byte(left), []byte(right))
}

func decodePublicKey(value string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, ErrInvalidInput
	}
	return decoded, nil
}

func validDeviceName(value string) bool {
	return value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 128
}

func validPlatform(value string) bool {
	switch value {
	case "windows", "darwin", "linux":
		return true
	default:
		return false
	}
}

func validProfileResource(value string) bool {
	return contains([]string{"models", "model-capabilities", "permission-profiles", "apps", "mcp", "plugins", "auth-status"}, value)
}

func validWorkspaceResource(value string) bool {
	return contains([]string{"sessions", "skills", "hooks"}, value)
}

func validUserPublicID(value string) bool {
	return validHex(strings.TrimSpace(value), 32)
}

func validPublicID(value, prefix string) bool {
	value = strings.TrimSpace(value)
	want := prefix + "_"
	return strings.HasPrefix(value, want) && validHex(strings.TrimPrefix(value, want), 32)
}

func validHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func newPublicID(prefix string) string {
	return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func deviceView(item domainagent.Device, userPublicID string) DeviceView {
	latest := buildinfo.ResolveVersion()
	return DeviceView{
		DeviceID: item.PublicID, UserID: userPublicID, Name: item.Name, Platform: item.Platform,
		AgentVersion: item.AgentVersion, LatestAgentVersion: latest,
		UpdateAvailable: validAgentVersion(item.AgentVersion) && validAgentVersion(latest) && compareAgentVersions(item.AgentVersion, latest) < 0,
		Status:          item.Status, Online: item.Online, LastSeenAt: item.LastSeenAt, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func validAgentVersion(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" || len(part) > 9 {
			return false
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return true
}

func compareAgentVersions(left, right string) int {
	leftParts, rightParts := strings.Split(left, "."), strings.Split(right, ".")
	for index := 0; index < 3; index++ {
		leftValue, _ := strconv.ParseUint(leftParts[index], 10, 32)
		rightValue, _ := strconv.ParseUint(rightParts[index], 10, 32)
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
	}
	return 0
}
