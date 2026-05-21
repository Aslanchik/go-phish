package telemetry

// TracerName is the instrumentation library name, matching the module path per OTel convention.
const TracerName = "github.com/aslanchik/go-phish"

// GenAI semconv — drawn from OTel semantic conventions v1.41.0.
const (
	AttrGenAIOperationName                 = "gen_ai.operation.name"
	AttrGenAIProviderName                  = "gen_ai.provider.name"
	AttrGenAIRequestModel                  = "gen_ai.request.model"
	AttrGenAIRequestMaxTokens              = "gen_ai.request.max_tokens"
	AttrGenAIRequestTemperature            = "gen_ai.request.temperature"
	AttrGenAIRequestTopP                   = "gen_ai.request.top_p"
	AttrGenAIRequestTopK                   = "gen_ai.request.top_k"
	AttrGenAIRequestStream                 = "gen_ai.request.stream"
	AttrGenAIResponseModel                 = "gen_ai.response.model"
	AttrGenAIResponseID                    = "gen_ai.response.id"
	AttrGenAIResponseFinishReasons         = "gen_ai.response.finish_reasons"
	AttrGenAIUsageInputTokens              = "gen_ai.usage.input_tokens"
	AttrGenAIUsageOutputTokens             = "gen_ai.usage.output_tokens"
	AttrGenAIUsageCacheCreationInputTokens = "gen_ai.usage.cache_creation.input_tokens"
	AttrGenAIUsageCacheReadInputTokens     = "gen_ai.usage.cache_read.input_tokens"
	AttrGenAIToolName                      = "gen_ai.tool.name"
	AttrGenAIToolCallID                    = "gen_ai.tool.call.id"
)

// ssspy extension namespace.
const (
	AttrInvestigationID       = "ssspy.investigation.id"
	AttrTargetURL             = "ssspy.investigation.target_url"
	AttrPhase                 = "ssspy.investigation.phase"
	AttrPhaseIndex            = "ssspy.investigation.phase_index"
	AttrPhaseOutcome          = "ssspy.investigation.outcome"
	AttrAgentName             = "ssspy.agent.name"
	AttrAgentVersion          = "ssspy.agent.version"
	AttrToolInput             = "ssspy.tool.input"
	AttrToolOutput            = "ssspy.tool.output"
	AttrScreenshotContentType = "ssspy.screenshot.content_type"
	AttrScreenshotSizeBytes   = "ssspy.screenshot.size_bytes"
	AttrScreenshotSHA256      = "ssspy.screenshot.sha256"
)
