package db

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// New 初始化 PostgreSQL 连接并执行迁移与种子数据。
func New(cfg config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.PostgresDSN), newGORMConfig(cfg))
	if err != nil {
		return nil, err
	}
	if err = configureTracing(db, cfg); err != nil {
		return nil, err
	}
	if err = configureConnectionPool(db, cfg); err != nil {
		return nil, err
	}

	if err = migrate(db, cfg); err != nil {
		return nil, err
	}
	if err = schema.SeedModelVendors(db); err != nil {
		return nil, err
	}

	if err = schema.SeedPermissionGroups(db); err != nil {
		return nil, err
	}

	return db, nil
}

func newGORMConfig(cfg config.Config) *gorm.Config {
	gormConfig := &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	}
	if isProductionEnv(cfg.Env) {
		gormConfig.Logger = productionGORMLogger()
	}
	return gormConfig
}

func productionGORMLogger() gormlogger.Interface {
	return gormlogger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), gormlogger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  gormlogger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
	})
}

func isProductionEnv(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}

func configureConnectionPool(db *gorm.DB, cfg config.Config) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	maxOpen := cfg.PostgresMaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 30
	}
	maxIdle := cfg.PostgresMaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 10
	}
	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)

	if cfg.PostgresConnMaxLifetimeMin > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.PostgresConnMaxLifetimeMin) * time.Minute)
	}
	if cfg.PostgresConnMaxIdleTimeMin > 0 {
		sqlDB.SetConnMaxIdleTime(time.Duration(cfg.PostgresConnMaxIdleTimeMin) * time.Minute)
	}
	return nil
}

func migrate(db *gorm.DB, cfg config.Config) error {
	if err := ensureCleanSlateSchema(db); err != nil {
		return err
	}
	if err := prepareUnifiedConversationExecution(db); err != nil {
		return err
	}
	if err := applySchemaBaseline(db); err != nil {
		return err
	}

	tableComments := map[string]string{
		"identity_users":                 "用户账户主表",
		"identity_sessions":              "用户登录会话表",
		"identity_auth_events":           "用户认证事件表",
		"agent_devices":                  "本地 Agent 设备表",
		"agent_credentials":              "Agent 一次性凭据哈希表",
		"agent_commands":                 "Agent 设备下行命令表",
		"agent_bridge_frames":            "Agent 网关上行终态帧表",
		"agent_runtime_profiles":         "Agent 本地运行时验证状态表",
		"agent_runtime_proof_challenges": "Agent 运行时一次性证明挑战表",
		"agent_workspaces":               "Agent 工作区投影表",
		"agent_resource_snapshots":       "Agent 本地资源快照表",
		"agent_threads":                  "Agent 工作会话表",
		"agent_turns":                    "Agent 工作回合表",
		"agent_events":                   "Agent 工作事件投影表",
		"agent_interactions":             "Agent 待处理交互表",
		"agent_idempotency_records":      "Agent HTTP 幂等记录表",
		"sub2_key_bindings":              "Sub2 API 密钥绑定表",
		"sub2_key_binding_operations":    "Sub2 API 密钥绑定幂等操作表",
		"sub2_payment_operations":        "Sub2 支付幂等操作表",
		"llm_upstreams":                  "上游配置表",
		"llm_upstream_models":            "上游真实模型清单表",
		"llm_model_vendors":              "平台模型技术厂商目录表",
		"llm_model_display_groups":       "平台模型自定义展示分组表",
		"llm_platform_models":            "平台模型表",
		"llm_model_routes":               "平台模型路由绑定表",
		"mcp_servers":                    "MCP服务配置表",
		"mcp_tools":                      "MCP工具发现表",
		"chat_conversations":             "聊天会话表",
		"chat_conversation_projects":     "会话项目分组表",
		"chat_conversation_shares":       "会话公开分享快照表",
		"chat_messages":                  "会话消息表",
		"chat_feedback":                  "会话消息反馈表",
		"chat_attachments":               "多模态附件元信息表",
		"file_objects":                   "文件对象与处理结果表",
		"file_storage_quotas":            "用户文件配额表",
		"chat_runs":                      "会话运行日志表",
		"chat_run_events":                "会话运行轨迹与工具事件表",
		"chat_context_records":           "会话上下文快照与证据表",
		"user_memories":                  "用户长期个性化记忆表",
		"audit_logs":                     "可追溯审计日志表",
		"system_events":                  "后台系统事件表",
		"system_announcements":           "站点公告表",
		"announcement_user_states":       "用户公告展示状态表",
		"prompt_presets":                 "内置与用户自定义预制提示词表",
		"skills":                         "内置与用户自定义技能提示词表",
		"system_settings":                "系统动态配置表",
		"user_settings":                  "用户个人偏好配置表",
		"file_chunks":                    "RAG文件分片表",
		"chat_message_chunks":            "会话消息向量分片表(历史对话语义检索)",
	}
	tableComments["chat_conversation_project_mcp_tools"] = "项目默认 MCP 工具关联表"
	tableComments["chat_conversation_project_skills"] = "项目默认 Skill 关联表"

	for table, comment := range tableComments {
		statement := fmt.Sprintf(`COMMENT ON TABLE "%s" IS '%s'`, table, escapeSQLLiteral(comment))
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}

	if err := applyConversationBaselineIndexes(db); err != nil {
		return err
	}
	if err := applyConversationExecutionConstraints(db); err != nil {
		return err
	}
	if err := applyLLMBaselineIndexes(db); err != nil {
		return err
	}
	if err := applyAnnouncementBaseline(db); err != nil {
		return err
	}
	if err := schema.CleanupRemovedColumns(db); err != nil {
		return err
	}
	if err := applyVectorBaseline(db); err != nil {
		return err
	}
	if err := schema.SeedLLMSettings(db); err != nil {
		return err
	}

	return nil
}

func applyConversationExecutionConstraints(db *gorm.DB) error {
	for _, statement := range []string{
		`DO $migration$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_chat_conversations_execution_type') THEN
				ALTER TABLE chat_conversations ADD CONSTRAINT chk_chat_conversations_execution_type CHECK (execution_type IN ('cloud', 'gateway'));
			END IF;
		END $migration$`,
		`DO $migration$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_chat_conversations_execution_target') THEN
				ALTER TABLE chat_conversations ADD CONSTRAINT chk_chat_conversations_execution_target CHECK (
					(execution_type = 'cloud' AND execution_device_id = '' AND execution_profile_id = '' AND execution_workspace_id = '')
					OR
					(execution_type = 'gateway' AND execution_device_id <> '' AND execution_profile_id <> '' AND execution_workspace_id <> '')
				);
			END IF;
		END $migration$`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func prepareUnifiedConversationExecution(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if tx.Migrator().HasTable(&model.AgentThread{}) && !tx.Migrator().HasColumn(&model.AgentThread{}, "conversation_id") {
			hasRows, err := tableHasRows(tx, model.AgentThread{}.TableName())
			if err != nil {
				return err
			}
			if hasRows {
				return fmt.Errorf("unified conversation cutover requires empty agent_threads before adding conversation ownership")
			}
		}
		if tx.Migrator().HasTable(&model.AgentTurn{}) && !tx.Migrator().HasColumn(&model.AgentTurn{}, "run_id") {
			hasRows, err := tableHasRows(tx, model.AgentTurn{}.TableName())
			if err != nil {
				return err
			}
			if hasRows {
				return fmt.Errorf("unified conversation cutover requires empty agent_turns before adding run ownership")
			}
		}
		if !tx.Migrator().HasTable(&model.Conversation{}) {
			return nil
		}
		for _, statement := range []string{
			`ALTER TABLE chat_conversations ADD COLUMN IF NOT EXISTS execution_type varchar(16)`,
			`ALTER TABLE chat_conversations ADD COLUMN IF NOT EXISTS execution_device_id varchar(64)`,
			`ALTER TABLE chat_conversations ADD COLUMN IF NOT EXISTS execution_profile_id varchar(64)`,
			`ALTER TABLE chat_conversations ADD COLUMN IF NOT EXISTS execution_workspace_id varchar(64)`,
			`ALTER TABLE chat_conversations ADD COLUMN IF NOT EXISTS execution_event_seq bigint`,
			`UPDATE chat_conversations SET execution_type = 'cloud' WHERE execution_type IS NULL OR execution_type = ''`,
			`UPDATE chat_conversations SET execution_device_id = '' WHERE execution_device_id IS NULL`,
			`UPDATE chat_conversations SET execution_profile_id = '' WHERE execution_profile_id IS NULL`,
			`UPDATE chat_conversations SET execution_workspace_id = '' WHERE execution_workspace_id IS NULL`,
			`UPDATE chat_conversations SET execution_event_seq = 0 WHERE execution_event_seq IS NULL`,
			`ALTER TABLE chat_conversations ALTER COLUMN execution_type DROP DEFAULT`,
			`ALTER TABLE chat_conversations ALTER COLUMN execution_type SET NOT NULL`,
			`ALTER TABLE chat_conversations ALTER COLUMN execution_device_id SET DEFAULT ''`,
			`ALTER TABLE chat_conversations ALTER COLUMN execution_device_id SET NOT NULL`,
			`ALTER TABLE chat_conversations ALTER COLUMN execution_profile_id SET DEFAULT ''`,
			`ALTER TABLE chat_conversations ALTER COLUMN execution_profile_id SET NOT NULL`,
			`ALTER TABLE chat_conversations ALTER COLUMN execution_workspace_id SET DEFAULT ''`,
			`ALTER TABLE chat_conversations ALTER COLUMN execution_workspace_id SET NOT NULL`,
			`ALTER TABLE chat_conversations ALTER COLUMN execution_event_seq SET DEFAULT 0`,
			`ALTER TABLE chat_conversations ALTER COLUMN execution_event_seq SET NOT NULL`,
		} {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

var removedCleanSlateTables = []string{
	"identity_contact_verifications",
	"identity_credentials",
	"identity_providers",
	"identity_user_links",
	"identity_mfa_settings",
	"identity_trusted_devices",
	"billing_plans",
	"billing_prices",
	"billing_subscriptions",
	"billing_payment_orders",
	"billing_accounts",
	"billing_balance_transactions",
	"billing_usage_reservations",
	"billing_redemption_codes",
	"billing_redemptions",
	"billing_model_prices",
	"billing_usage_ledgers",
}

func ensureCleanSlateSchema(db *gorm.DB) error {
	for _, check := range []struct {
		table   string
		columns []string
	}{
		{table: "identity_users", columns: []string{"sub2_instance_id", "sub2_user_id"}},
		{table: "identity_sessions", columns: []string{"sub2_access_token_encrypted", "sub2_refresh_token_encrypted", "sub2_access_expires_at", "sub2_verified_at"}},
	} {
		hasRows, err := tableHasRows(db, check.table)
		if err != nil {
			return err
		}
		if !hasRows {
			continue
		}
		for _, column := range check.columns {
			hasColumn, err := tableHasColumn(db, check.table, column)
			if err != nil {
				return err
			}
			if !hasColumn {
				return fmt.Errorf("clean-slate database required: populated table %s is missing %s", check.table, column)
			}
		}
	}

	for _, table := range removedCleanSlateTables {
		hasRows, err := tableHasRows(db, table)
		if err != nil {
			return err
		}
		if hasRows {
			return fmt.Errorf("clean-slate database required: removed table %s still contains data", table)
		}
	}
	return nil
}

func tableHasRows(db *gorm.DB, table string) (bool, error) {
	var exists bool
	if err := db.Raw("SELECT to_regclass(?) IS NOT NULL", table).Scan(&exists).Error; err != nil || !exists {
		return false, err
	}
	var hasRows bool
	err := db.Raw(fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %q LIMIT 1)`, table)).Scan(&hasRows).Error
	return hasRows, err
}

func tableHasColumn(db *gorm.DB, table, column string) (bool, error) {
	var exists bool
	err := db.Raw(`SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?
	)`, table, column).Scan(&exists).Error
	return exists, err
}

func applySchemaBaseline(db *gorm.DB) error {
	return schema.Migrate(db)
}

func escapeSQLLiteral(input string) string {
	return strings.ReplaceAll(input, "'", "''")
}

func applyLLMBaselineIndexes(db *gorm.DB) error {
	statements := []string{
		`ALTER TABLE "llm_upstreams"
		ADD COLUMN IF NOT EXISTS "protocol_defaults_json" text NOT NULL DEFAULT '{}'`,
		`COMMENT ON COLUMN "llm_upstreams"."protocol_defaults_json" IS '按模型类型配置的默认协议JSON'`,
		`ALTER TABLE "llm_platform_models"
		ADD COLUMN IF NOT EXISTS "system_prompt" text NOT NULL DEFAULT ''`,
		`COMMENT ON COLUMN "llm_platform_models"."system_prompt" IS '模型级系统提示词'`,
		`ALTER TABLE "llm_platform_models"
		ADD COLUMN IF NOT EXISTS "access_scope" varchar(32) NOT NULL DEFAULT 'public'`,
		`COMMENT ON COLUMN "llm_platform_models"."access_scope" IS '模型使用范围: public用户可用 internal仅内部任务'`,
		`CREATE INDEX IF NOT EXISTS idx_llm_platform_models_access_scope
			ON "llm_platform_models" ("access_scope")`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_upstream_models_upstream_name
			ON "llm_upstream_models" ("upstream_id", "upstream_model_name")`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_upstream_models_binding_code
			ON "llm_upstream_models" ("binding_code")`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_platform_models_name
			ON "llm_platform_models" ("name")`,
		`DROP INDEX IF EXISTS idx_llm_model_routes_unique`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_model_routes_unique
			ON "llm_model_routes" ("platform_model_id", "upstream_model_id", "protocol")`,
		`CREATE INDEX IF NOT EXISTS idx_llm_model_routes_routing
			ON "llm_model_routes" ("platform_model_id", "status", "priority", "weight")
			WHERE status = 'active'`,
	}

	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func applyAnnouncementBaseline(db *gorm.DB) error {
	statements := []string{
		`ALTER TABLE "system_announcements"
		ADD COLUMN IF NOT EXISTS "type" varchar(32) NOT NULL DEFAULT 'general'`,
		`COMMENT ON COLUMN "system_announcements"."type" IS '公告类型(critical/warning/info/normal/general)'`,
		`ALTER TABLE "system_announcements"
		ADD COLUMN IF NOT EXISTS "pinned" boolean NOT NULL DEFAULT false`,
		`COMMENT ON COLUMN "system_announcements"."pinned" IS '是否置顶'`,
		`CREATE INDEX IF NOT EXISTS idx_system_announcements_sort
		ON "system_announcements" ("pinned", "priority", "updated_at", "id")`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_announcement_user_states_version
		ON "announcement_user_states" ("announcement_id", "user_id", "announcement_updated_at")`,
		`ALTER TABLE "announcement_user_states"
		ADD COLUMN IF NOT EXISTS "closed_at" timestamptz`,
		`COMMENT ON COLUMN "announcement_user_states"."closed_at" IS '关闭时间'`,
		`CREATE INDEX IF NOT EXISTS idx_announcement_user_states_user_dismissed
		ON "announcement_user_states" ("user_id", "dismissed_until")
		WHERE "dismissed_until" IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_announcement_user_states_user_closed
		ON "announcement_user_states" ("user_id", "closed_at")
		WHERE "closed_at" IS NOT NULL`,
	}

	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func applyConversationBaselineIndexes(db *gorm.DB) error {
	statements := []string{
		`ALTER TABLE "chat_conversations"
		ADD COLUMN IF NOT EXISTS "project_id" bigint`,
		`COMMENT ON COLUMN "chat_conversations"."project_id" IS '项目分组ID'`,
		`ALTER TABLE "chat_conversation_projects"
		ADD COLUMN IF NOT EXISTS "system_prompt" text NOT NULL DEFAULT ''`,
		`COMMENT ON COLUMN "chat_conversation_projects"."system_prompt" IS '项目级系统提示词'`,
		`CREATE INDEX IF NOT EXISTS idx_chat_conversations_user_status_starred_updated_at
		ON "chat_conversations" ("user_id", "status", "is_starred", "updated_at" DESC, "id" DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_conversations_user_status_starred_starred_at
		ON "chat_conversations" ("user_id", "status", "is_starred", "starred_at" DESC, "id" DESC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_conversation_projects_public_id
		ON "chat_conversation_projects" ("public_id")
		WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_chat_conversation_projects_user_status_sort
		ON "chat_conversation_projects" ("user_id", "status", "sort_order" ASC, "id" DESC)
		WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_chat_conversations_user_project_status_updated
		ON "chat_conversations" ("user_id", "project_id", "status", "updated_at" DESC, "id" DESC)
		WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_chat_conversation_shares_active_conversation
		ON "chat_conversation_shares" ("conversation_id", "updated_at" DESC, "id" DESC)
		WHERE status = 'active'`,
		`CREATE INDEX IF NOT EXISTS idx_chat_conversation_shares_user_status_updated_at
		ON "chat_conversation_shares" ("user_id", "status", "updated_at" DESC, "id" DESC)`,
		`ALTER TABLE "chat_messages"
		ADD COLUMN IF NOT EXISTS "reasoning_content" text NOT NULL DEFAULT ''`,
		`COMMENT ON COLUMN "chat_messages"."reasoning_content" IS '上游推理内容回灌上下文'`,
		`ALTER TABLE "chat_messages"
		ADD COLUMN IF NOT EXISTS "edited_at" timestamptz`,
		`COMMENT ON COLUMN "chat_messages"."edited_at" IS '用户编辑时间'`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_edited_at
		ON "chat_messages" ("edited_at")`,
		`ALTER TABLE "chat_runs"
		ADD COLUMN IF NOT EXISTS "task_type" varchar(32) NOT NULL DEFAULT 'chat'`,
		`COMMENT ON COLUMN "chat_runs"."task_type" IS '任务类型'`,
		`CREATE INDEX IF NOT EXISTS idx_chat_runs_task_type
		ON "chat_runs" ("task_type")`,
		`ALTER TABLE "chat_context_records"
		ADD COLUMN IF NOT EXISTS "covered_until_message_id" bigint NOT NULL DEFAULT 0`,
		`COMMENT ON COLUMN "chat_context_records"."covered_until_message_id" IS '快照覆盖到的最后消息ID'`,
		`ALTER TABLE "chat_context_records"
		ADD COLUMN IF NOT EXISTS "covered_until_public_id" varchar(32) NOT NULL DEFAULT ''`,
		`COMMENT ON COLUMN "chat_context_records"."covered_until_public_id" IS '快照覆盖到的最后消息公开ID'`,
		`ALTER TABLE "chat_context_records"
		ADD COLUMN IF NOT EXISTS "coverage_path_hash" varchar(64) NOT NULL DEFAULT ''`,
		`COMMENT ON COLUMN "chat_context_records"."coverage_path_hash" IS '快照覆盖分支路径Hash'`,
		`ALTER TABLE "chat_context_records"
		ADD COLUMN IF NOT EXISTS "covered_message_count" integer NOT NULL DEFAULT 0`,
		`COMMENT ON COLUMN "chat_context_records"."covered_message_count" IS '快照覆盖消息数'`,
		`CREATE INDEX IF NOT EXISTS idx_chat_context_records_covered_until_message_id
		ON "chat_context_records" ("covered_until_message_id")`,
		`CREATE INDEX IF NOT EXISTS idx_chat_context_records_covered_until_public_id
		ON "chat_context_records" ("covered_until_public_id")`,
		`CREATE INDEX IF NOT EXISTS idx_chat_context_records_coverage_path_hash
		ON "chat_context_records" ("coverage_path_hash")`,
		`ALTER TABLE "chat_run_events"
		ALTER COLUMN "event_id" TYPE varchar(255),
		ALTER COLUMN "parent_event_id" TYPE varchar(255),
		ALTER COLUMN "title" TYPE varchar(255),
		ALTER COLUMN "tool_call_id" TYPE varchar(255)`,
		`COMMENT ON COLUMN "chat_run_events"."event_id" IS '事件ID'`,
		`COMMENT ON COLUMN "chat_run_events"."parent_event_id" IS '父事件ID'`,
		`COMMENT ON COLUMN "chat_run_events"."title" IS '轨迹标题'`,
		`COMMENT ON COLUMN "chat_run_events"."tool_call_id" IS '工具调用ID'`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_file_objects_active_user_content
		ON "file_objects" ("user_id", "sha256", "size_bytes")
		WHERE status = 'active' AND deleted_at IS NULL AND sha256 <> ''`,
	}

	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}

	return nil
}

// applyVectorBaseline 确保 pgvector 扩展、向量列和检索索引存在。
func applyVectorBaseline(db *gorm.DB) error {
	if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS vector`).Error; err != nil {
		return fmt.Errorf("create pgvector extension: %w", err)
	}

	statements := []struct {
		name string
		sql  string
	}{
		{
			name: "add file_chunks embedding column",
			sql:  `ALTER TABLE "file_chunks" ADD COLUMN IF NOT EXISTS embedding vector(1536)`,
		},
		{
			name: "index file_chunks embedding",
			sql: `CREATE INDEX IF NOT EXISTS idx_file_chunks_embedding
				ON "file_chunks" USING ivfflat (embedding vector_cosine_ops)
				WITH (lists = 100)`,
		},
		{
			name: "add chat_message_chunks embedding column",
			sql:  `ALTER TABLE "chat_message_chunks" ADD COLUMN IF NOT EXISTS embedding vector(1536)`,
		},
		{
			name: "index chat_message_chunks embedding",
			sql: `CREATE INDEX IF NOT EXISTS idx_chat_message_chunks_embedding
				ON "chat_message_chunks" USING ivfflat (embedding vector_cosine_ops)
				WITH (lists = 100)`,
		},
		{
			name: "add user_memories embedding column",
			sql:  `ALTER TABLE "user_memories" ADD COLUMN IF NOT EXISTS embedding vector(1536)`,
		},
		{
			name: "index user_memories embedding",
			sql: `CREATE INDEX IF NOT EXISTS idx_user_memories_embedding
				ON "user_memories" USING ivfflat (embedding vector_cosine_ops)
				WITH (lists = 50)`,
		},
	}

	for _, statement := range statements {
		if err := db.Exec(statement.sql).Error; err != nil {
			return fmt.Errorf("%s: %w", statement.name, err)
		}
	}
	return nil
}
