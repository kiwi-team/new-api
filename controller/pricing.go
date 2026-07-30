package controller

import (
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		if common.StringsContains(item.EnableGroup, "all") {
			filtered = append(filtered, item)
			continue
		}
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

func GetPricing(c *gin.Context) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	for s, f := range ratio_setting.GetGroupRatioCopy() {
		groupRatio[s] = f
	}
	var group string
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
			for g := range groupRatio {
				ratio, ok := ratio_setting.GetGroupGroupRatio(group, g)
				if ok {
					groupRatio[g] = ratio
				}
			}
		}
	}

	usableGroup = service.GetUserUsableGroups(group)
	pricing = filterPricingByUsableGroups(pricing, usableGroup)
	// check groupRatio contains usableGroup
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
		}
	}

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            model.GetVendors(),
		"group_ratio":        groupRatio,
		"usable_group":       usableGroup,
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        service.GetUserAutoGroup(group),
		"pricing_version":    "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}

// modelPriceUpdateTimeOption 存储每个模型价格的最近修改时间（unix 秒）。
// 形如 {"gpt-4o": 1783500000}，仅由价格中心的行内编辑维护，供「修改时间」列展示。
const modelPriceUpdateTimeOption = "ModelPriceUpdateTime"

// saveModelPriceMap 将某个价格 map 持久化到 option 并热更新运行时倍率表。
func saveModelPriceMap(key string, m map[string]float64) error {
	b, err := common.Marshal(m)
	if err != nil {
		return err
	}
	return model.UpdateOption(key, string(b))
}

// touchModelPriceUpdateTime 记录/更新某模型价格的修改时间。
func touchModelPriceUpdateTime(modelName string, ts int64) error {
	common.OptionMapRWMutex.RLock()
	cur := common.OptionMap[modelPriceUpdateTimeOption]
	common.OptionMapRWMutex.RUnlock()
	tsMap := map[string]int64{}
	if cur != "" {
		_ = common.UnmarshalJsonStr(cur, &tsMap)
	}
	tsMap[modelName] = ts
	b, err := common.Marshal(tsMap)
	if err != nil {
		return err
	}
	return model.UpdateOption(modelPriceUpdateTimeOption, string(b))
}

type updateModelPricingRequest struct {
	ModelName        string  `json:"model_name"`
	IsPerCall        bool    `json:"is_per_call"`        // true=按次计费
	InputPrice       float64 `json:"input_price"`        // 按量：输入 $/1M tokens
	OutputPrice      float64 `json:"output_price"`       // 按量：输出 $/1M tokens
	PerCallPrice     float64 `json:"per_call_price"`     // 按次：$/次
	CacheReadPrice   float64 `json:"cache_read_price"`   // 按量：缓存读取 $/1M tokens
	CacheCreatePrice float64 `json:"cache_create_price"` // 按量：缓存创建 $/1M tokens
}

// UpdateModelPricing PUT /api/pricing/model —— 行内即时保存单个模型的官方价格。
// 前端传美金价，后端换算成倍率（ratio = 输入价/2，completion = 输出价/输入价，
// cacheRatio = 缓存读取价/输入价，createCacheRatio = 缓存创建价/输入价）并落库，
// 同时记录修改时间。root only。
func UpdateModelPricing(c *gin.Context) {
	var req updateModelPricingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	req.ModelName = strings.TrimSpace(req.ModelName)
	if req.ModelName == "" {
		c.JSON(200, gin.H{"success": false, "message": "模型名称不能为空"})
		return
	}
	if req.InputPrice < 0 || req.OutputPrice < 0 || req.PerCallPrice < 0 ||
		req.CacheReadPrice < 0 || req.CacheCreatePrice < 0 {
		c.JSON(200, gin.H{"success": false, "message": "价格不能为负数"})
		return
	}

	ratioMap := ratio_setting.GetModelRatioCopy()
	completionMap := ratio_setting.GetCompletionRatioCopy()
	priceMap := ratio_setting.GetModelPriceCopy()
	cacheMap := ratio_setting.GetCacheRatioCopy()
	createCacheMap := ratio_setting.GetCreateCacheRatioCopy()

	if req.IsPerCall {
		priceMap[req.ModelName] = req.PerCallPrice
		delete(ratioMap, req.ModelName)
		delete(completionMap, req.ModelName)
		delete(cacheMap, req.ModelName)
		delete(createCacheMap, req.ModelName)
	} else {
		ratio := req.InputPrice / 2
		completion := 1.0
		if req.InputPrice > 0 {
			completion = req.OutputPrice / req.InputPrice
		}
		ratioMap[req.ModelName] = ratio
		completionMap[req.ModelName] = completion
		delete(priceMap, req.ModelName)

		// 缓存价格换算为倍率（相对输入价）；输入价为 0 或缓存价为 0 时不配置对应倍率
		if req.InputPrice > 0 && req.CacheReadPrice > 0 {
			cacheMap[req.ModelName] = req.CacheReadPrice / req.InputPrice
		} else {
			delete(cacheMap, req.ModelName)
		}
		if req.InputPrice > 0 && req.CacheCreatePrice > 0 {
			createCacheMap[req.ModelName] = req.CacheCreatePrice / req.InputPrice
		} else {
			delete(createCacheMap, req.ModelName)
		}
	}

	for key, m := range map[string]map[string]float64{
		"ModelRatio":       ratioMap,
		"CompletionRatio":  completionMap,
		"ModelPrice":       priceMap,
		"CacheRatio":       cacheMap,
		"CreateCacheRatio": createCacheMap,
	} {
		if err := saveModelPriceMap(key, m); err != nil {
			c.JSON(200, gin.H{"success": false, "message": err.Error()})
			return
		}
	}

	now := time.Now().Unix()
	if err := touchModelPriceUpdateTime(req.ModelName, now); err != nil {
		c.JSON(200, gin.H{"success": false, "message": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    gin.H{"updated_at": now},
	})
}

// DeleteModelPricing DELETE /api/pricing/model?model_name= —— 删除某模型的官方价格。root only。
func DeleteModelPricing(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model_name"))
	if modelName == "" {
		c.JSON(200, gin.H{"success": false, "message": "模型名称不能为空"})
		return
	}
	ratioMap := ratio_setting.GetModelRatioCopy()
	completionMap := ratio_setting.GetCompletionRatioCopy()
	priceMap := ratio_setting.GetModelPriceCopy()
	cacheMap := ratio_setting.GetCacheRatioCopy()
	createCacheMap := ratio_setting.GetCreateCacheRatioCopy()
	delete(ratioMap, modelName)
	delete(completionMap, modelName)
	delete(priceMap, modelName)
	delete(cacheMap, modelName)
	delete(createCacheMap, modelName)
	for key, m := range map[string]map[string]float64{
		"ModelRatio":       ratioMap,
		"CompletionRatio":  completionMap,
		"ModelPrice":       priceMap,
		"CacheRatio":       cacheMap,
		"CreateCacheRatio": createCacheMap,
	} {
		if err := saveModelPriceMap(key, m); err != nil {
			c.JSON(200, gin.H{"success": false, "message": err.Error()})
			return
		}
	}
	// 同步清理修改时间记录
	common.OptionMapRWMutex.RLock()
	cur := common.OptionMap[modelPriceUpdateTimeOption]
	common.OptionMapRWMutex.RUnlock()
	if cur != "" {
		tsMap := map[string]int64{}
		if err := common.UnmarshalJsonStr(cur, &tsMap); err == nil {
			delete(tsMap, modelName)
			if b, err := common.Marshal(tsMap); err == nil {
				_ = model.UpdateOption(modelPriceUpdateTimeOption, string(b))
			}
		}
	}
	c.JSON(200, gin.H{"success": true, "message": ""})
}
