package controller

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/service"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func GetAllQuotaDates(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	defaultTime := c.Query("default_time")
	if defaultTime == "" {
		defaultTime = "hour"
	}
	dates, err := model.GetAllQuotaDates(startTimestamp, endTimestamp, username, defaultTime)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

func GetUserQuotaDates(c *gin.Context) {
	userId := c.GetInt("id")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 判断时间跨度是否超过 1 个月
	if endTimestamp-startTimestamp > 2592000 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "时间跨度不能超过 1 个月",
		})
		return
	}
	defaultTime := c.Query("default_time")
	if defaultTime == "" {
		defaultTime = "hour"
	}
	dates, err := model.GetQuotaDataByUserId(userId, startTimestamp, endTimestamp, defaultTime)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

var prevWarningTimeMap = make(map[int]int64)

func WarningUserQuota() {
	if !common.QuotaWarningEnabled {
		return
	}
	webhookUrl := common.OptionMap["FeishuRobotUrl"]
	secret := common.OptionMap["FeishuRobotSecret"]
	for {
		time.Sleep(time.Duration(common.QuotaWarningInterval) * time.Minute)
		now := time.Now().Unix() - int64(common.DataExportInterval)*3*60 // 3分钟之前
		ctx := context.TODO()

		userIds := strings.Split(common.QuotaWarningUserIds, ",")
		for _, userIdStr := range userIds {
			userId, err := strconv.Atoi(userIdStr)
			fileName := fmt.Sprintf("quota_warning_%d.txt", userId)
			if err != nil {
				logger.LogError(ctx, "error parsing user id: "+err.Error())
				continue
			}
			prevWarningTime, ok := prevWarningTimeMap[userId]
			if !ok {

				// 从fileName中读取上次预警时间
				data, err1 := os.ReadFile(fileName)
				if err1 != nil {
					logger.LogError(ctx, "error reading file: "+err1.Error())
					continue
				}
				prevWarningTime, err = strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
				if err != nil {
					logger.LogError(ctx, "error parsing file data: "+err.Error())
					continue
				}
				prevWarningTimeMap[userId] = prevWarningTime
			}
			loc, _ := time.LoadLocation("Asia/Shanghai")
			startTimeStr := time.Unix(prevWarningTime, 0).In(loc).Format("2006-01-02 15:04:05")
			endTimeStr := time.Unix(now, 0).In(loc).Format("2006-01-02 15:04:05")
			common.SysLog("[" + startTimeStr + "~" + endTimeStr + "]" + ":" + ":消耗预警轮询开始")

			quota, err := model.GetQuotaByTime(userId, prevWarningTime, now)
			if err != nil {
				logger.LogError(ctx, "error getting quota: "+err.Error())
				continue
			}
			// 加上内存缓存中尚未落库的消耗
			//cachedQuota := model.GetCachedQuotaByUser(userId, prevWarningTime, now)
			//quota += cachedQuota
			dollerQuota := int(float64(quota) / common.QuotaPerUnit)
			if dollerQuota >= common.QuotaWarningThreshold {
				content := fmt.Sprintf("用户 %d 在%s ~ %s 内消耗了 %d 美元额度", userId, startTimeStr, endTimeStr, dollerQuota)
				err = service.SendFeishuNotify(webhookUrl, secret, dto.FeishuNotify{
					MsgType: "text",
					Content: dto.FeishuContent{
						Text: content,
					},
				})
				if err != nil {
					logger.LogError(ctx, "error sending webhook notify: "+err.Error())
					continue
				}
				common.SysLog("消耗预警:" + content)
				prevWarningTimeMap[userId] = now

				// 将 now 的值写入文件
				err = os.WriteFile(fileName, []byte(strconv.FormatInt(now, 10)), 0644)
				if err != nil {
					logger.LogError(ctx, "error writing now to file: "+err.Error())
				}
			} else {
				common.SysLog("[" + startTimeStr + "~" + endTimeStr + "]" + ":" + strconv.Itoa(dollerQuota) + ":未超过阈值")
			}
		}
	}
}

func GetQuotaDataStatistics(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	modelName := c.Query("model_name")
	clientUserId := c.Query("client_user_id")
	clientUserId = strings.ReplaceAll(clientUserId, " ", "+")
	clientScenairos := c.Query("client_scenairos")
	expandModels := c.Query("expand_models") == "true"
	expandDates := c.Query("expand_dates") == "true"
	expandTokens := c.Query("expand_tokens") == "true"
	userId, _ := strconv.Atoi(c.Query("user_id"))
	projectName := c.Query("project_name")
	tokenIdsStr := c.Query("token_ids")

	// 普通用户（通过 MixRouterAuth）只能查看自己或其关联 uid 的数据
	scopeUserId, scopeUids := resolveSelfScope(c)
	if scopeUserId > 0 {
		userId = 0 // 改用 scope 限制，不再按精确 userId 过滤
	}

	var tokenIds []int
	if tokenIdsStr != "" {
		for _, idStr := range strings.Split(tokenIdsStr, ",") {
			idStr = strings.TrimSpace(idStr)
			if id, err := strconv.Atoi(idStr); err == nil {
				tokenIds = append(tokenIds, id)
			}
		}
	}

	statistics, err := model.GetQuotaDataStatistics(startTimestamp, endTimestamp, modelName, clientUserId, clientScenairos, expandModels, expandDates, expandTokens, userId, projectName, tokenIds, scopeUserId, scopeUids)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    statistics,
	})
}

// resolveSelfScope 当请求被标记为 force_self_user_id（非管理员自助视图）时，
// 返回当前用户 id 及其可见 uid 集合（自身 uid + 关联 uid）。否则返回 0/nil。
func resolveSelfScope(c *gin.Context) (int, []string) {
	if forceSelf, exists := c.Get("force_self_user_id"); !exists || !forceSelf.(bool) {
		return 0, nil
	}
	selfId := c.GetInt("id")
	if selfId <= 0 {
		return 0, nil
	}
	if u, err := model.GetUserById(selfId, false); err == nil {
		return selfId, u.GetScopeUids()
	}
	return selfId, nil
}

func ExportQuotaDataStatistics(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	modelName := c.Query("model_name")
	clientUserId := c.Query("client_user_id")
	clientUserId = strings.ReplaceAll(clientUserId, " ", "+")
	clientScenairos := c.Query("client_scenairos")
	expandModels := c.Query("expand_models") == "true"
	expandDates := c.Query("expand_dates") == "true"
	expandTokens := c.Query("expand_tokens") == "true"
	userId, _ := strconv.Atoi(c.Query("user_id"))
	projectName := c.Query("project_name")
	tokenIdsStr := c.Query("token_ids")

	// 普通用户（通过 MixRouterAuth）只能导出自己或其关联 uid 的数据
	scopeUserId, scopeUids := resolveSelfScope(c)
	if scopeUserId > 0 {
		userId = 0
	}

	var tokenIds []int
	if tokenIdsStr != "" {
		for _, idStr := range strings.Split(tokenIdsStr, ",") {
			idStr = strings.TrimSpace(idStr)
			if id, err := strconv.Atoi(idStr); err == nil {
				tokenIds = append(tokenIds, id)
			}
		}
	}

	statistics, err := model.GetQuotaDataStatistics(startTimestamp, endTimestamp, modelName, clientUserId, clientScenairos, expandModels, expandDates, expandTokens, userId, projectName, tokenIds, scopeUserId, scopeUids)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Generate CSV
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment;filename=quota_statistics_%d_%d.csv", startTimestamp, endTimestamp))

	writer := csv.NewWriter(c.Writer)
	// Build CSV header dynamically
	header := []string{}
	if expandDates {
		header = append(header, "Date")
	}
	header = append(header, "Client User ID")
	if expandModels {
		header = append(header, "Model Name")
	}
	if expandTokens {
		header = append(header, "Token ID", "Token Name", "Token Key")
	}
	header = append(header, "Total Count", "Total Quota", "Total Prompt Tokens", "Total Completion Tokens",
		"Cached Tokens", "Cache Cost",
		"Cache Creation Tokens", "Cache Creation Cost",
		"Cache Creation 5m Tokens", "Cache Creation 5m Cost",
		"Cache Creation 1h Tokens", "Cache Creation 1h Cost")
	if !expandModels {
		header = append(header, "Fixed Budget", "Temp Budget")
	}
	writer.Write(header)

	for _, stat := range statistics {
		row := []string{}
		if expandDates {
			row = append(row, stat.Date)
		}
		row = append(row, stat.ClientUserId)
		if expandModels {
			row = append(row, stat.ModelName)
		}
		if expandTokens {
			row = append(row, strconv.Itoa(stat.TokenId), stat.TokenName, stat.TokenKey)
		}
		row = append(row,
			strconv.FormatInt(stat.TotalCount, 10),
			strconv.FormatFloat(stat.TotalQuota, 'f', 2, 64),
			strconv.FormatInt(stat.TotalPrompt, 10),
			strconv.FormatInt(stat.TotalCompletion, 10),
			strconv.FormatInt(stat.TotalCachedTokens, 10),
			strconv.FormatFloat(stat.TotalCacheCost, 'f', 6, 64),
			strconv.FormatInt(stat.TotalCacheCreationTokens, 10),
			strconv.FormatFloat(stat.TotalCacheCreationCost, 'f', 6, 64),
			strconv.FormatInt(stat.TotalCacheCreation5mTokens, 10),
			strconv.FormatFloat(stat.TotalCacheCreation5mCost, 'f', 6, 64),
			strconv.FormatInt(stat.TotalCacheCreation1hTokens, 10),
			strconv.FormatFloat(stat.TotalCacheCreation1hCost, 'f', 6, 64),
		)
		if !expandModels {
			row = append(row, strconv.Itoa(stat.FixedQuota), strconv.Itoa(stat.TempQuota))
		}
		writer.Write(row)
	}
	writer.Flush()
}

// GetChannelQuotaStatistics 获取渠道消耗统计数据
func GetChannelQuotaStatistics(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	statistics, err := model.GetChannelQuotaStatistics(startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    statistics,
	})
}

// GetDistinctProjectNames 获取所有不重复的项目名称
func GetDistinctProjectNames(c *gin.Context) {
	names, err := model.GetDistinctProjectNames()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    names,
	})
}

// GetTokenListForStatistics 获取token列表用于消耗统计页面的下拉筛选
// root用户可以获取所有token，非root用户只能获取自己的token
func GetTokenListForStatistics(c *gin.Context) {
	role := c.GetInt("role")
	userId := c.GetInt("id")

	var queryUserId int
	if role >= common.RoleRootUser {
		// root用户：可以获取所有token
		queryUserId = 0
	} else {
		// 非root用户：只能获取自己的token
		queryUserId = userId
	}

	tokens, err := model.GetTokenListForDropdown(queryUserId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    tokens,
	})
}
