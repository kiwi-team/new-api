package dto

// {"gpt-4o-mini":{"retry":2,"disable_channels":[10,23],"channels":[{"id":23,"weight":10,"group_ratio":{"default":1,"zero":0}},{"id":24,"weight":10}]}}
type ChannelRulesItem struct {
	Retry           int           `json:"retry,omitempty"`
	DisableChannels []int         `json:"disable_channels,omitempty"`
	Channels        []ChannelItem `json:"channels,omitempty"`
}

type ChannelItem struct {
	GroupRatio map[string]float64 `json:"group_ratio,omitempty"`
	Name       string             `json:"name,omitempty"` // 名称可以是正则表达式，减少配置成本
	Id         int                `json:"id,omitempty"`
	Weight     int32              `json:"weight,omitempty"`
}
