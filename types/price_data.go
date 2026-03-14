package types

import "fmt"

type GroupRatioInfo struct {
	GroupRatio             float64
	GroupSpecialRatio      float64
	HasSpecialRatio        bool
	UserGroupDiscount      float64 // 用户分组折扣
	UserModelExtraDiscount float64 // 用户模型额外折扣
	OriginalGroupRatio     float64 // 折扣前的分组倍率
}

type PriceData struct {
	FreeModel            bool
	ModelPrice           float64
	ModelRatio           float64
	CompletionRatio      float64
	CacheRatio           float64
	CacheCreationRatio   float64
	CacheCreation5mRatio float64
	CacheCreation1hRatio float64
	ImageRatio           float64
	AudioRatio           float64
	AudioCompletionRatio float64
	OtherRatios          map[string]float64
	UsePrice             bool
	QuotaToPreConsume    int // 预消耗额度
	GroupRatioInfo       GroupRatioInfo

	// 阶梯价格相关字段
	UseTieredPrice    bool    // 是否使用阶梯价格
	TieredInputPrice  float64 // 匹配到的档位输入价格（每百万 Token）
	TieredOutputPrice float64 // 匹配到的档位输出价格（每百万 Token）
	TieredMaxTokens   int     // 匹配到的档位阈值
}

func (p *PriceData) AddOtherRatio(key string, ratio float64) {
	if p.OtherRatios == nil {
		p.OtherRatios = make(map[string]float64)
	}
	if ratio <= 0 {
		return
	}
	p.OtherRatios[key] = ratio
}

type PerCallPriceData struct {
	ModelPrice     float64
	Quota          int
	GroupRatioInfo GroupRatioInfo
}

func (p *PriceData) ToSetting() string {
	return fmt.Sprintf("ModelPrice: %f, ModelRatio: %f, CompletionRatio: %f, CacheRatio: %f, GroupRatio: %f, UsePrice: %t, CacheCreationRatio: %f, CacheCreation5mRatio: %f, CacheCreation1hRatio: %f, QuotaToPreConsume: %d, ImageRatio: %f, AudioRatio: %f, AudioCompletionRatio: %f, UseTieredPrice: %t, TieredInputPrice: %f, TieredOutputPrice: %f, TieredMaxTokens: %d", p.ModelPrice, p.ModelRatio, p.CompletionRatio, p.CacheRatio, p.GroupRatioInfo.GroupRatio, p.UsePrice, p.CacheCreationRatio, p.CacheCreation5mRatio, p.CacheCreation1hRatio, p.QuotaToPreConsume, p.ImageRatio, p.AudioRatio, p.AudioCompletionRatio, p.UseTieredPrice, p.TieredInputPrice, p.TieredOutputPrice, p.TieredMaxTokens)
}
