package ratio_setting

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
)

// https://platform.minimaxi.com/docs/guides/pricing#%E8%AF%AD%E9%9F%B3%E8%B5%84%E6%BA%90%E5%8C%85 ¥700/2,000,000 字符
// https://help.aliyun.com/zh/model-studio/qwen-tts?spm=a2c4g.11186623.0.0.26e12b19qDqr2k  0.8元/万字符
// https://console.volcengine.com/speech/service/10035/buy-word?AppID=1921851494&ActiveID=volc.seedtts.default 28元/10万字符
const (
	// minimx 3.5 元/万字符
	// aliyun 0.8元/万字符
	// volcengine 2.8元/万字符

	// default 2元/万字符  0.02/千字符

	// 1 === $0.002 / 1K字符
	// 1 === ￥0.014 / 1k字符
	DefaultTTSRatio = 10
)

var DefaultModelTTSRatio = map[string]float64{
	"speech-2.5-hd-preview": 175,
	"qwen3-tts-flash":       40,
	"seed-tts-2.0":          140,
	"eleven_v3":             100,
}

func GetTTSRatio(model string) float64 {
	ttsRatio := common.OptionMap["tts_ratio"]
	ttsRationMap := make(map[string]float64)
	json.Unmarshal([]byte(ttsRatio), &ttsRationMap)
	if ratio, ok := ttsRationMap[model]; ok {
		return ratio
	}
	ratio, ok := DefaultModelTTSRatio[model]
	if !ok {
		return DefaultTTSRatio
	}
	return ratio
}
