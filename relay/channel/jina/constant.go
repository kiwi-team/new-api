package jina

var ModelList = []string{
	"jina-clip-v1",
	"jina-reranker-v2-base-multilingual",
	"jina-reranker-m0",
	// 占位模型：仅用于 /v1/jina/reader 路径的 channel 池匹配，jina 端不需要。
	"jina-reader",
}

var ChannelName = "jina"
