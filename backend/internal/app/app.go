package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/admin"
	appagentgateway "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/agentgateway"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/announcement"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/audit"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/auth"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/channel"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/compact"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/conversation"
	appembedding "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/embedding"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/extraction"
	applogcleanup "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/logcleanup"
	appmcp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/mcp"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/memory"
	appstorage "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/objectstorage"
	appprocessing "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/processing"
	apppromptpreset "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/promptpreset"
	apprag "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/rag"
	appruntime "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/runtime"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/settings"
	appskill "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/skill"
	appsub2commerce "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/sub2commerce"
	appsub2key "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/sub2key"
	appsystemevent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/systemevent"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/usersettings"
	domainagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/agentgateway"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/embedding"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/geoip"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/llm"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/mcp"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/mediaartifact"
	platformlogger "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/observability/logger"
	platformtracing "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/observability/tracing"
	agentgatewayrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/agentgateway"
	announcementrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/announcement"
	auditrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/audit"
	channelrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/channel"
	conversationrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/conversation"
	logcleanuprepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/logcleanup"
	mcprepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/mcp"
	memoryrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/memory"
	promptpresetrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/promptpreset"
	settingsrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/settings"
	skillrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/skill"
	sub2commercerepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/sub2commerce"
	sub2keyrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/sub2key"
	systemeventrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/systemevent"
	userrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/user"
	usersettingsrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/usersettings"
	platformruntime "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/runtime"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/sub2api"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/buildinfo"
	platformhttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http"
	adminhttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/admin"
	agentgatewayhttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/agentgateway"
	announcementhttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/announcement"
	authhttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/auth"
	billinghttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/billing"
	channelhttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/channel"
	conversationhttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/conversation"
	mcphttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/mcp"
	memoryhttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/memory"
	promptpresethttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/promptpreset"
	settingshttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/settings"
	skillhttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/skill"
	sub2keyhttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/sub2key"
	userhttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/user"
	usersettingshttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/usersettings"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/update"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// App 维护应用运行依赖。
type App struct {
	cfg                 config.Config
	engine              *gin.Engine
	logger              *zap.Logger
	db                  *gorm.DB
	redis               *redis.Client
	geoResolver         *geoip.Client
	llmClient           *llm.Client
	mcpClient           *mcp.Client
	embeddingClient     *embedding.Client
	mediaArtifactClient *mediaartifact.Client
	backgroundCancel    context.CancelFunc
}

type avatarContentOpener struct {
	conversationService *conversation.Service
}

type agentArtifactStore struct {
	conversationService *conversation.Service
}

type localGatewayAdapter struct{ service *appagentgateway.Service }

func mapLocalGatewayError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, appagentgateway.ErrInvalidInput):
		return conversation.ErrInvalidExecutionTarget
	case errors.Is(err, appagentgateway.ErrResourceNotFound), errors.Is(err, appagentgateway.ErrDeviceNotFound):
		return conversation.ErrExecutionBindingNotFound
	case errors.Is(err, appagentgateway.ErrStateConflict):
		return conversation.ErrExecutionConflict
	default:
		return err
	}
}

func (a localGatewayAdapter) ResolveExecutionTarget(ctx context.Context, userID uint, deviceID, profileID, workspaceID string) (string, error) {
	provider, err := a.service.ResolveExecutionTarget(ctx, userID, deviceID, profileID, workspaceID)
	return provider, mapLocalGatewayError(err)
}

func (a localGatewayAdapter) CreateArtifact(ctx context.Context, userID uint, workspaceID, fileID string) (*conversation.GatewayArtifact, error) {
	item, err := a.service.CreateArtifact(ctx, userID, workspaceID, fileID)
	if err != nil {
		return nil, mapLocalGatewayError(err)
	}
	return &conversation.GatewayArtifact{ArtifactID: item.ArtifactID}, nil
}

func (a localGatewayAdapter) StartThread(ctx context.Context, userID uint, input conversation.GatewayStartThreadInput) error {
	_, err := a.service.StartThread(ctx, userID, appagentgateway.StartThreadInput{
		DeviceID: input.DeviceID, ProfileID: input.ProfileID, WorkspaceID: input.WorkspaceID,
		ConversationID: input.ConversationID, Title: input.Title, Settings: input.Settings,
		InitialInput: input.InitialInput, InitialRunID: input.InitialRunID, IdempotencyKey: input.IdempotencyKey,
	})
	return mapLocalGatewayError(err)
}

func (a localGatewayAdapter) GetThreadByConversation(ctx context.Context, userID, conversationID uint) (*conversation.GatewayThread, error) {
	item, err := a.service.GetThreadByConversation(ctx, userID, conversationID)
	if err != nil {
		if errors.Is(err, appagentgateway.ErrResourceNotFound) {
			return nil, conversation.ErrExecutionBindingNotFound
		}
		return nil, mapLocalGatewayError(err)
	}
	return &conversation.GatewayThread{ThreadID: item.ThreadID}, nil
}

func (a localGatewayAdapter) StartTurn(ctx context.Context, userID uint, input conversation.GatewayStartTurnInput) error {
	_, err := a.service.StartTurn(ctx, userID, appagentgateway.StartTurnInput{
		ThreadID: input.ThreadID, RunID: input.RunID, IdempotencyKey: input.IdempotencyKey,
		Input: input.Input, Settings: input.Settings,
	})
	return mapLocalGatewayError(err)
}

func (a localGatewayAdapter) InterruptRun(ctx context.Context, userID uint, runID, idempotencyKey string) error {
	_, err := a.service.InterruptRun(ctx, userID, runID, idempotencyKey)
	return mapLocalGatewayError(err)
}

func (a localGatewayAdapter) ListInteractions(ctx context.Context, userID, conversationID uint, status string) ([]conversation.GatewayInteraction, error) {
	thread, err := a.service.GetThreadByConversation(ctx, userID, conversationID)
	if err != nil {
		if errors.Is(err, appagentgateway.ErrResourceNotFound) {
			return []conversation.GatewayInteraction{}, nil
		}
		return nil, mapLocalGatewayError(err)
	}
	items, err := a.service.ListInteractions(ctx, userID, thread.ThreadID, status)
	if err != nil {
		if errors.Is(err, appagentgateway.ErrInvalidInput) {
			return nil, conversation.ErrInvalidInteraction
		}
		return nil, mapLocalGatewayError(err)
	}
	result := make([]conversation.GatewayInteraction, 0, len(items))
	for _, item := range items {
		result = append(result, conversation.GatewayInteraction{
			InteractionID: item.InteractionID, RunID: item.RunID, Kind: item.Kind,
			Status: item.Status, Request: item.Request, CreatedAt: item.CreatedAt,
		})
	}
	return result, nil
}

func (a localGatewayAdapter) RespondInteraction(ctx context.Context, userID uint, input conversation.GatewayInteractionResponse) (*conversation.GatewayInteraction, error) {
	item, err := a.service.RespondInteraction(ctx, userID, appagentgateway.RespondInteractionInput{
		InteractionID: input.InteractionID, IdempotencyKey: input.IdempotencyKey, Response: input.Response,
	})
	if err != nil {
		switch {
		case errors.Is(err, appagentgateway.ErrInvalidInput):
			return nil, conversation.ErrInvalidInteraction
		case errors.Is(err, appagentgateway.ErrResourceNotFound):
			return nil, conversation.ErrInteractionNotFound
		case errors.Is(err, appagentgateway.ErrStateConflict):
			return nil, conversation.ErrExecutionConflict
		default:
			return nil, err
		}
	}
	return &conversation.GatewayInteraction{
		InteractionID: item.InteractionID, RunID: item.RunID, Kind: item.Kind,
		Status: item.Status, Request: item.Request, CreatedAt: item.CreatedAt,
	}, nil
}

func projectLocalGatewayEvent(target *conversation.Service) appagentgateway.ConversationEventProjector {
	return func(ctx context.Context, item domainagent.AppliedEventFrame) error {
		return target.ProjectGatewayEvent(ctx, conversation.GatewayExecutionEvent{
			SourceKey: "agent:" + item.Event.PublicID, UserID: item.Event.UserID,
			ConversationID: item.ConversationID, RunID: item.RunID, Kind: item.Event.Kind,
			Payload: []byte(item.Event.PayloadJSON), OccurredAt: item.Event.OccurredAt,
		})
	}
}

func (s agentArtifactStore) OpenAgentArtifact(ctx context.Context, userID uint, fileID string) (*appagentgateway.ArtifactContent, error) {
	content, err := s.conversationService.OpenFileContent(ctx, userID, fileID)
	if err != nil {
		return nil, err
	}
	return &appagentgateway.ArtifactContent{Reader: content.Reader, ContentType: content.ContentType, SizeBytes: content.SizeBytes}, nil
}

func (o avatarContentOpener) OpenAvatarFileContent(ctx context.Context, userID uint, fileID string) (*user.AvatarFileContent, error) {
	content, err := o.conversationService.OpenFileContent(ctx, userID, fileID)
	if err != nil {
		return nil, err
	}
	return &user.AvatarFileContent{
		Reader:      content.Reader,
		ContentType: content.ContentType,
		SizeBytes:   content.SizeBytes,
		ModTime:     content.ModTime,
		FileName:    content.File.FileName,
	}, nil
}

// NewApp 创建应用。
func NewApp() (*App, error) {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	runtimeCfg := config.NewRuntime(cfg)

	if err := platformtracing.Init(context.Background(), platformtracing.Config{
		ServiceName:  cfg.AppName,
		Enabled:      cfg.OTelEnabled,
		Endpoint:     cfg.OTelExporterOTLPEndpoint,
		Headers:      cfg.OTelExporterOTLPHeaders,
		Insecure:     cfg.OTelExporterOTLPInsecure,
		Protocol:     cfg.OTelExporterOTLPProtocol,
		SamplingRate: cfg.OTelSamplingRate,
	}); err != nil {
		return nil, fmt.Errorf("init tracing: %w", err)
	}

	log, err := platformlogger.New(cfg.Env)
	if err != nil {
		return nil, err
	}

	db, err := openDatabase(cfg)
	if err != nil {
		return nil, err
	}

	redisClient, err := openCache(cfg)
	if err != nil {
		return nil, err
	}

	auditRepo := auditrepo.NewRepo(db)
	auditService := audit.NewService(auditRepo, log)
	logCleanupRepo := logcleanuprepo.NewRepo(db)
	logCleanupService := applogcleanup.NewService(logCleanupRepo, auditService)
	systemEventRepo := systemeventrepo.NewRepo(db)
	systemEventService := appsystemevent.NewService(systemEventRepo)

	// 初始化 settings 模块：种子数据 + 动态配置覆盖
	settingsRepo := settingsrepo.NewRepo(db)
	settingsService := settings.NewService(settingsRepo, cfg.DataEncryptionKey)
	settingsService.SetAuditWriter(auditService)
	runtimeService := appruntime.NewService(runtimeCfg)
	runtimeService.SetDockerRunner(platformruntime.NewDockerRunner())
	settingsCache := buildSettingsCache(redisClient)
	runtimeSettings := settings.NewRuntimeSettings(settingsRepo, settingsCache, cfg.DataEncryptionKey)
	settingsHandler := settingshttp.NewHandler(settingsService, runtimeSettings, runtimeService, runtimeCfg)
	settingsModule := settingshttp.NewModule(settingsHandler)
	if err = settingsService.Seed(context.Background(), cfg); err != nil {
		return nil, fmt.Errorf("seed settings: %w", err)
	}
	if err = runtimeSettings.ApplyTo(context.Background(), runtimeCfg); err != nil {
		return nil, fmt.Errorf("apply settings: %w", err)
	}

	// 启动时确保 embedding_model_signature 已写入：首次部署或签名字段为空时自动补全。
	if startCfg := runtimeCfg.Snapshot(); startCfg.EmbeddingModelSignature == "" && startCfg.RAGModel != "" {
		initialSig := appembedding.ComputeModelSignature(startCfg.RAGModel, startCfg.EmbeddingOutputDimensions)
		if _, seedErr := settingsService.BatchUpdate(context.Background(), []settings.PatchItem{
			{Namespace: "file", Key: "embedding_model_signature", Value: initialSig},
		}); seedErr == nil {
			_ = runtimeSettings.ApplyTo(context.Background(), runtimeCfg)
		}
	}

	userRepo := userrepo.NewRepo(db)
	userService := user.NewService(userRepo)
	agentGatewayService, err := appagentgateway.NewService(agentgatewayrepo.NewRepo(db), cfg.DataEncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("init agent gateway service: %w", err)
	}
	agentGatewayModule := agentgatewayhttp.NewModule(agentgatewayhttp.NewHandler(agentGatewayService))
	var billingModule *billinghttp.Module
	objectStoreProvider := appstorage.NewRuntimeProvider(runtimeCfg, nil)
	geoResolver := geoip.New(runtimeCfg.Snapshot())
	sub2Client, err := sub2api.New(cfg.Sub2BaseURL, cfg.StrictOutboundPolicy())
	if err != nil {
		return nil, fmt.Errorf("init Sub2API client: %w", err)
	}
	authService, err := auth.NewServiceWithRuntime(
		runtimeCfg,
		userRepo,
		geoResolver,
		sub2Client,
	)
	if err != nil {
		return nil, fmt.Errorf("init auth service: %w", err)
	}
	authService.SetLogger(log)
	authService.SetObjectStoreProvider(objectStoreProvider)
	authService.SetAuditWriter(auditService)
	authHandler := authhttp.NewHandler(authService)
	authModule := authhttp.NewModule(authHandler)
	billingModule = billinghttp.NewSub2Module(billinghttp.NewSub2Handler(appsub2commerce.NewService(authService, sub2Client, sub2commercerepo.NewRepo(db))))
	sub2KeyService := appsub2key.NewService(sub2keyrepo.NewRepo(db), authService, sub2Client, cfg.DataEncryptionKey)
	agentGatewayService.SetRuntimeAuth(authService, sub2KeyService)
	sub2KeyModule := sub2keyhttp.NewModule(sub2keyhttp.NewHandler(sub2KeyService))
	memoryRepo := memoryrepo.NewRepo(db)
	memoryService := memory.NewService(memoryRepo)
	memoryService.SetAuditWriter(auditService)
	memoryHandler := memoryhttp.NewHandler(memoryService)
	memoryModule := memoryhttp.NewModule(memoryHandler)
	channelRepo := channelrepo.NewRepo(db)
	channelCache := buildChannelCache(redisClient)
	trustedOutboundPolicy := cfg.TrustedOutboundPolicy()
	strictOutboundPolicy := cfg.StrictOutboundPolicy()
	llmClient := llm.NewClient(trustedOutboundPolicy)
	mcpClient := mcp.NewClient(trustedOutboundPolicy)
	mediaArtifactClient := mediaartifact.New(strictOutboundPolicy)
	channelService := channel.NewServiceWithRuntime(runtimeCfg, channelRepo, channelRepo, channelCache, llmClient)
	channelService.SetLogger(log)
	channelService.SetPermissionGroupRepo(channelRepo)
	settingsHandler.SetNativeToolCatalogProvider(channelService)
	channelHandler := channelhttp.NewHandler(channelService)
	channelModule := channelhttp.NewModule(channelHandler)
	conversationRepo := conversationrepo.NewRepo(db)
	settingsService.SetVectorStoreAvailabilityService(conversationRepo)
	conversationCache := buildConversationCache(redisClient)
	mcpRepo := mcprepo.NewRepo(db)
	embedClient := embedding.New(trustedOutboundPolicy)
	compactService := compact.NewServiceWithRuntime(runtimeCfg, conversationRepo, log)
	extractionService := extraction.NewServiceWithRuntime(runtimeCfg)
	extractionService.SetObjectStoreProvider(objectStoreProvider)
	embeddingService := appembedding.NewServiceWithRuntime(runtimeCfg, conversationRepo, extractionService, embedClient, log)
	memoryService.SetEmbeddingProvider(embeddingService)
	settingsHandler.SetEmbeddingService(embeddingService)
	processingService := appprocessing.NewServiceWithRuntime(runtimeCfg, conversationRepo, conversationCache, extractionService, embeddingService, log, appprocessing.DefaultExtractorVersion)
	ragService := apprag.NewServiceWithRuntime(runtimeCfg, conversationRepo, conversationCache, embedClient)
	conversationService := conversation.NewServiceWithRuntime(
		runtimeCfg,
		conversationRepo,
		conversationCache,
		channelService,
		memoryService,
		llmClient,
		mediaArtifactClient,
		mcpClient,
		embedClient,
		nil,
		compactService,
		embeddingService,
		processingService,
		extractionService,
		ragService,
		log,
	)
	conversationService.SetSub2ExecutionResolver(sub2KeyService)
	conversationService.SetGatewayExecutor(localGatewayAdapter{service: agentGatewayService})
	agentGatewayService.SetConversationEventProjector(projectLocalGatewayEvent(conversationService))
	conversationService.SetAuditWriter(auditService)
	conversationService.SetObjectStoreProvider(objectStoreProvider)
	conversationService.SetMCPRepository(mcpRepo)
	agentGatewayService.SetArtifactContentStore(agentArtifactStore{conversationService: conversationService})
	userService.SetAvatarContentOpener(avatarContentOpener{conversationService: conversationService})
	authService.SetAvatarFileValidator(conversationService)
	memoryService.SetCacheInvalidator(conversationService.InvalidateMemoryCache)
	conversationHandler := conversationhttp.NewHandler(conversationService, runtimeCfg)
	conversationModule := conversationhttp.NewModule(conversationHandler)
	userHandler := userhttp.NewHandler(userService)
	userModule := userhttp.NewModule(userHandler)
	mcpService := appmcp.NewServiceWithRuntime(runtimeCfg, mcpRepo, mcpClient)
	mcpService.SetSystemEventWriter(systemEventService)
	mcpHandler := mcphttp.NewHandler(mcpService)
	mcpModule := mcphttp.NewModule(mcpHandler)
	adminService := admin.NewService(userService, auditService)
	adminService.SetSystemEventService(systemEventService)
	adminService.SetConversationEventService(conversationService)
	adminService.SetLogCleanupService(logCleanupService)
	adminService.SetPermissionGroupRepo(channelRepo)
	adminService.SetPermissionGroupModelLookup(channelRepo)
	adminHandler := adminhttp.NewHandler(adminService)
	applicationUpdater, err := update.NewUpdater(update.Config{
		Repository:      cfg.UpdateRepository,
		RuntimeDir:      cfg.UpdateRuntimeDir,
		StateFile:       cfg.UpdateStateFile,
		CurrentVersion:  buildinfo.ResolveVersion(),
		ProxyURL:        cfg.UpdateProxyURL,
		DownloadTimeout: time.Duration(cfg.UpdateDownloadTimeoutSeconds) * time.Second,
		Restart:         func() { os.Exit(0) },
	})
	if err != nil {
		return nil, fmt.Errorf("init updater: %w", err)
	}
	adminHandler.SetUpdater(applicationUpdater)
	adminHandler.SetConversationExporter(conversationService)
	adminModule := adminhttp.NewModule(adminHandler)
	userSettingsRepo := usersettingsrepo.NewRepo(db)
	userSettingsService := usersettings.NewService(userSettingsRepo)
	userSettingsService.SetCacheInvalidator(conversationService.InvalidateUserSettingCache)
	userSettingsHandler := usersettingshttp.NewHandler(userSettingsService)
	userSettingsModule := usersettingshttp.NewModule(userSettingsHandler)
	announcementRepo := announcementrepo.NewRepo(db)
	announcementService := announcement.NewService(announcementRepo, authService, sub2Client)
	announcementHandler := announcementhttp.NewHandler(announcementService)
	announcementModule := announcementhttp.NewModule(announcementHandler)
	promptPresetRepo := promptpresetrepo.NewRepo(db)
	promptPresetService := apppromptpreset.NewService(promptPresetRepo)
	promptPresetService.SetAuditWriter(auditService)
	promptPresetHandler := promptpresethttp.NewHandler(promptPresetService)
	promptPresetModule := promptpresethttp.NewModule(promptPresetHandler)
	skillRepo := skillrepo.NewRepo(db)
	skillService := appskill.NewService(skillRepo)
	skillService.SetAuditWriter(auditService)
	conversationService.SetSkillResolver(skillService)
	skillHandler := skillhttp.NewHandler(skillService)
	skillModule := skillhttp.NewModule(skillHandler)

	hc := newHealthChecker(db, redisClient)
	rateLimiter := buildRateLimiter(redisClient)
	engine, err := platformhttp.NewEngine(runtimeCfg, log, platformhttp.Modules{
		AgentGateway: agentGatewayModule,
		Auth:         authModule,
		AuthService:  authService,
		Channel:      channelModule,
		Conversation: conversationModule,
		MCP:          mcpModule,
		Memory:       memoryModule,
		Billing:      billingModule,
		Sub2Key:      sub2KeyModule,
		Admin:        adminModule,
		Announcement: announcementModule,
		PromptPreset: promptPresetModule,
		Skill:        skillModule,
		Settings:     settingsModule,
		UserSettings: userSettingsModule,
		User:         userModule,
	}, hc, rateLimiter)
	if err != nil {
		return nil, err
	}

	backgroundCtx, backgroundCancel := context.WithCancel(context.Background())
	conversationService.StartBackgroundWorkers(backgroundCtx)

	return &App{
		cfg:                 runtimeCfg.Snapshot(),
		engine:              engine,
		logger:              log,
		db:                  db,
		redis:               redisClient,
		geoResolver:         geoResolver,
		llmClient:           llmClient,
		mcpClient:           mcpClient,
		embeddingClient:     embedClient,
		mediaArtifactClient: mediaArtifactClient,
		backgroundCancel:    backgroundCancel,
	}, nil
}

// Run 启动 HTTP 服务并支持优雅停机。
func (a *App) Run() error {
	addr := fmt.Sprintf(":%s", a.cfg.HTTPPort)
	srv := &http.Server{
		Addr:              addr,
		Handler:           a.engine,
		ReadHeaderTimeout: httpTimeoutSeconds(a.cfg.HTTPReadHeaderTimeoutSeconds, 10),
		ReadTimeout:       httpTimeoutSeconds(a.cfg.HTTPReadTimeoutSeconds, 120),
		IdleTimeout:       httpTimeoutSeconds(a.cfg.HTTPIdleTimeoutSeconds, 120),
		MaxHeaderBytes:    httpMaxHeaderBytes(a.cfg.HTTPMaxHeaderBytes),
	}

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("server_starting", zap.String("port", a.cfg.HTTPPort))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-quit:
		a.logger.Info("server_shutting_down", zap.String("signal", sig.String()))
	}

	if a.backgroundCancel != nil {
		a.backgroundCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		a.logger.Error("server_shutdown_error", zap.Error(err))
		return err
	}
	a.logger.Info("server_stopped")
	return nil
}

func httpTimeoutSeconds(value int, fallback int) time.Duration {
	if value <= 0 {
		value = fallback
	}
	return time.Duration(value) * time.Second
}

func httpMaxHeaderBytes(value int) int {
	if value <= 0 {
		return 1 << 20
	}
	return value
}

// Close 关闭资源。
func (a *App) Close() {
	if a.backgroundCancel != nil {
		a.backgroundCancel()
	}
	if a.redis != nil {
		_ = a.redis.Close()
	}
	if a.geoResolver != nil {
		a.geoResolver.Close()
	}
	if a.llmClient != nil {
		a.llmClient.CloseIdleConnections()
	}
	if a.mcpClient != nil {
		a.mcpClient.CloseIdleConnections()
	}
	if a.embeddingClient != nil {
		a.embeddingClient.CloseIdleConnections()
	}
	if a.mediaArtifactClient != nil {
		a.mediaArtifactClient.CloseIdleConnections()
	}
	if a.db != nil {
		if sqlDB, err := a.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	platformtracing.Shutdown(shutdownCtx)
	a.logger.Sync() //nolint:errcheck
}
