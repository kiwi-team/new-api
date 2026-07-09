package reve

// ModelList 是 Reve 渠道默认暴露的模型列表。
// 输出到上游的 `model` 字段即为此处的名称。
var ModelList = []string{
	"reve",
}

var ChannelName = "reve"

const (
	// Reve 文生图 / 图生图接口路径
	createImagePath = "/v1/image/create"
	editImagePath   = "/v1/image/edit"
)
