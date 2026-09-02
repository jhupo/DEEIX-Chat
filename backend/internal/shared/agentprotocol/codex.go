package agentprotocol

const (
	CodexMinimumRuntimeVersion = "0.151.0"
	CodexMaximumRuntimeMinor   = "0.151"
	CodexProtocolVersion       = "0.151.0/stable"
	CodexSchemaHash            = "424b204943b18e5ffa52667a2aa397c9950730ec1e49ad767e2a016743990541"
	SessionSnapshotEventKind   = "workspace/sessions/updated"
	SessionHistoryEventKind    = "thread/history/updated"
	MaxProviderEventBytes      = 2 << 20
	MaxTerminalOutcomeBytes    = 256 << 20
	MaxTerminalUploadBytes     = 64 << 20
	MaxSessionHistoryBytes     = 256 << 20
)
