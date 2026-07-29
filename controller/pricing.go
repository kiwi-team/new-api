package controller

import (
	"math"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

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
		"_":                  "a42d372ccf0b5dd13ecf71203521f9d2",
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

// 倍率消噪的容差。
//
// 价格中心前端提交的美金价已按 6 位小数量化，再经 ratio = 价/2、cacheRatio = 缓存价/输入价
// 这类除法后会残留浮点噪声（例如缓存倍率算出 0.100000136 而非 0.1），
// 这些噪声会原样落库并在页面上反显成 2.000002 之类的脏数字。
//
// 容差同时受相对与绝对两个上限约束，取更严的那个：
//   - 相对上限保证小数值不会被吸附到量级不同的数字上；
//   - 绝对上限锚定前端 6 位量化的步长，保证大数值不会产生肉眼可见的漂移
//     （只用相对上限时，倍率 73.529412 会被吸附成 73.529，反显就变成 ¥999.99）。
const (
	ratioSnapRelTolerance = 1e-5
	ratioSnapAbsTolerance = 1e-6
)

// snapRatio 消除倍率里的浮点噪声：在误差不超过容差的前提下，
// 把数值吸附到位数最短的等价小数（0.100000136 -> 0.1）。
//
// 注意这不是「保留 N 位小数」——误差超出容差时原样保留，
// 所以 0.075 不会被压成 0.08、0.001 不会被压成 0，实际计费不受影响。
func snapRatio(x float64) float64 {
	if x == 0 || math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	tol := math.Min(math.Abs(x)*ratioSnapRelTolerance, ratioSnapAbsTolerance)
	for d := 0; d <= 6; d++ {
		p := math.Pow(10, float64(d))
		c := math.Round(x*p) / p
		if c != 0 && math.Abs(c-x) <= tol {
			return c
		}
	}
	return x
}

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
		priceMap[req.ModelName] = snapRatio(req.PerCallPrice)
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
		ratioMap[req.ModelName] = snapRatio(ratio)
		completionMap[req.ModelName] = snapRatio(completion)
		delete(priceMap, req.ModelName)

		// 缓存价格换算为倍率（相对输入价）；输入价为 0 或缓存价为 0 时不配置对应倍率
		if req.InputPrice > 0 && req.CacheReadPrice > 0 {
			cacheMap[req.ModelName] = snapRatio(req.CacheReadPrice / req.InputPrice)
		} else {
			delete(cacheMap, req.ModelName)
		}
		if req.InputPrice > 0 && req.CacheCreatePrice > 0 {
			createCacheMap[req.ModelName] = snapRatio(req.CacheCreatePrice / req.InputPrice)
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
