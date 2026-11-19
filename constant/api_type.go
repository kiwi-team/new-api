package constant

const (
	APITypeOpenAI = iota
	APITypeAnthropic
	APITypePaLM
	APITypeBaidu
	APITypeZhipu
	APITypeAli
	APITypeXunfei
	APITypeAIProxyLibrary
	APITypeTencent
	APITypeGemini
	APITypeZhipuV4
	APITypeOllama
	APITypePerplexity
	APITypeAws
	APITypeCohere
	APITypeDify
	APITypeJina
	APITypeCloudflare
	APITypeSiliconFlow
	APITypeVertexAi
	APITypeMistral
	APITypeDeepSeek
	APITypeMokaAI
	APITypeVolcEngine
	APITypeBaiduV2
	APITypeOpenRouter
	APITypeXinference
	APITypeXai
	APITypeCoze
	APITypeSensenova
	APITypeVisualVolcEngine
	APITypeJimeng
	APITypeMoonshot
	APITypeSubmodel
	APITypeMiniMax
	APITypeElevenLabs
	APITypeAliDashScope
	APITypeFAL
	APITypeReplicate
	APITypeAwsV2
	APITypeFALSync
	APITypeCodex
	APITypeQwenRealtime
	APITypePPIO
	APITypeGeminiRealtime
	APITypeVolcEngineRealtime
	APITypeReve
	APITypeAdvancedCustom
	APITypeSub2API
	APITypeNewAPI
	APITypeTypeSafe
	// APITypeProviderPlugin serves a task plugin channel (type 61) whose
	// plugin has relay routes; ChannelType2APIType never returns it.
	APITypeProviderPlugin
	APITypeDummy // this one is only for count, do not add any channel after this
)
