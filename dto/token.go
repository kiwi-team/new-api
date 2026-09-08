package dto

// {"gpt-4o-mini":{"retry":2,"disable_channels":[10,23],"channels":[{"id":23,"weight":10,"group_ratio":{"default":1,"zero":0}},{"id":24,"weight":10}]}}
type ChannelRulesItem struct {
	Retry           int           `json:"retry,omitempty"`
	DisableChannels []int         `json:"disable_channels,omitempty"`
	Channels        []ChannelItem `json:"channels,omitempty"`
	RandomType      string        `json:"random_type,omitempty"` // random,order,race
	RaceTimeout     int           `json:"race_timeout,omitempty"`
}

const (
	ChannelRuleModeRace       = "race"
	DefaultRaceTimeoutSeconds = 25
	MinRaceTimeoutSeconds     = 1
	MaxRaceTimeoutSeconds     = 300
)

type ChannelRacePlan struct {
	Groups         [][]int
	TimeoutSeconds int
}

type ChannelItem struct {
	GroupRatio map[string]float64 `json:"group_ratio,omitempty"`
	Name       string             `json:"name,omitempty"` // 名称可以是正则表达式，减少配置成本
	Id         int                `json:"id,omitempty"`
	Ids        []int              `json:"ids,omitempty"` // 同一个级别的多个渠道，这些渠道的权重是相等的
	Weight     int32              `json:"weight,omitempty"`
}

type OnlyTextChannels struct {
	ModelName  string `json:"model_name,omitempty"`
	ChannelIds []int  `json:"channel_ids,omitempty"`
	RandomType string `json:"random_type,omitempty"` // random,order
}

// SpecialChannels 特殊渠道，比如不包含视频的渠道，和包含视频的渠道
type SpecailChannels struct {
	ModelName  string  `json:"model_name,omitempty"`
	ChannelIds [][]int `json:"channel_ids,omitempty"`
	RandomType string  `json:"random_type,omitempty"` // random,order
}
