package controller

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func checkRootOnly(c *gin.Context) bool {
	role := c.GetInt("role")
	if role != common.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "仅超级管理员可执行此操作",
		})
		return false
	}
	return true
}

// -------- Export --------

func ExportUsersCSV(c *gin.Context) {
	if !checkRootOnly(c) {
		return
	}
	var users []model.User
	if err := model.DB.Unscoped().Find(&users).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment;filename=users.csv")
	w := csv.NewWriter(c.Writer)
	w.Write([]string{
		"id", "username", "password", "display_name", "role", "status", "email",
		"github_id", "discord_id", "oidc_id", "wechat_id", "telegram_id",
		"group", "quota", "used_quota", "request_count", "aff_code", "aff_count",
		"aff_quota", "aff_history_quota", "inviter_id", "linux_do_id", "setting",
		"stripe_customer", "toio_registered",
	})
	for _, u := range users {
		w.Write([]string{
			strconv.Itoa(u.Id),
			u.Username,
			u.Password,
			u.DisplayName,
			strconv.Itoa(u.Role),
			strconv.Itoa(u.Status),
			u.Email,
			u.GitHubId,
			u.DiscordId,
			u.OidcId,
			u.WeChatId,
			u.TelegramId,
			u.Group,
			strconv.Itoa(u.Quota),
			strconv.Itoa(u.UsedQuota),
			strconv.Itoa(u.RequestCount),
			u.AffCode,
			strconv.Itoa(u.AffCount),
			strconv.Itoa(u.AffQuota),
			strconv.Itoa(u.AffHistoryQuota),
			strconv.Itoa(u.InviterId),
			u.LinuxDOId,
			u.Setting,
			u.StripeCustomer,
			strconv.Itoa(u.ToioRegistered),
		})
	}
	w.Flush()
}

func ExportTokensCSV(c *gin.Context) {
	if !checkRootOnly(c) {
		return
	}
	var tokens []model.Token
	if err := model.DB.Unscoped().Find(&tokens).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment;filename=tokens.csv")
	w := csv.NewWriter(c.Writer)
	w.Write([]string{
		"id", "user_id", "key", "status", "name", "created_time", "accessed_time",
		"expired_time", "remain_quota", "unlimited_quota", "model_limits_enabled",
		"model_limits", "allow_ips", "used_quota", "channel_rules", "channel_ratios",
		"group", "cross_group_retry", "channel_rules_high_priority",
	})
	for _, t := range tokens {
		allowIps := ""
		if t.AllowIps != nil {
			allowIps = *t.AllowIps
		}
		w.Write([]string{
			strconv.Itoa(t.Id),
			strconv.Itoa(t.UserId),
			t.Key,
			strconv.Itoa(t.Status),
			t.Name,
			strconv.FormatInt(t.CreatedTime, 10),
			strconv.FormatInt(t.AccessedTime, 10),
			strconv.FormatInt(t.ExpiredTime, 10),
			strconv.Itoa(t.RemainQuota),
			strconv.FormatBool(t.UnlimitedQuota),
			strconv.FormatBool(t.ModelLimitsEnabled),
			t.ModelLimits,
			allowIps,
			strconv.Itoa(t.UsedQuota),
			t.ChannelRules,
			t.ChannelRatios,
			t.Group,
			strconv.FormatBool(t.CrossGroupRetry),
			strconv.FormatBool(t.ChannelRulesHighPriority),
		})
	}
	w.Flush()
}

func ExportChannelsCSV(c *gin.Context) {
	if !checkRootOnly(c) {
		return
	}
	var channels []model.Channel
	if err := model.DB.Unscoped().Find(&channels).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment;filename=channels.csv")
	w := csv.NewWriter(c.Writer)
	w.Write([]string{
		"id", "type", "key", "name", "weight", "created_time", "base_url", "other",
		"status",
		"balance", "balance_updated_time", "models", "group", "used_quota",
		"model_mapping", "ratio", "remark", "status_code_mapping", "priority",
		"auto_ban", "other_info", "tag", "setting", "param_override", "header_override", "settings",
	})
	for _, ch := range channels {
		weight := ""
		if ch.Weight != nil {
			weight = strconv.Itoa(int(*ch.Weight))
		}
		baseURL := ""
		if ch.BaseURL != nil {
			baseURL = *ch.BaseURL
		}
		modelMapping := ""
		if ch.ModelMapping != nil {
			modelMapping = *ch.ModelMapping
		}
		ratio := ""
		if ch.Ratio != nil {
			ratio = fmt.Sprintf("%f", *ch.Ratio)
		}
		remark := ""
		if ch.Remark != nil {
			remark = *ch.Remark
		}
		statusCodeMapping := ""
		if ch.StatusCodeMapping != nil {
			statusCodeMapping = *ch.StatusCodeMapping
		}
		priority := ""
		if ch.Priority != nil {
			priority = strconv.Itoa(int(*ch.Priority))
		}
		autoBan := ""
		if ch.AutoBan != nil {
			autoBan = strconv.Itoa(*ch.AutoBan)
		}
		tag := ""
		if ch.Tag != nil {
			tag = *ch.Tag
		}
		setting := ""
		if ch.Setting != nil {
			setting = *ch.Setting
		}
		paramOverride := ""
		if ch.ParamOverride != nil {
			paramOverride = *ch.ParamOverride
		}
		headerOverride := ""
		if ch.HeaderOverride != nil {
			headerOverride = *ch.HeaderOverride
		}
		w.Write([]string{
			strconv.Itoa(ch.Id),
			strconv.Itoa(ch.Type),
			ch.Key,
			ch.Name,
			weight,
			strconv.FormatInt(ch.CreatedTime, 10),
			baseURL,
			ch.Other,
			strconv.Itoa(ch.Status),
			fmt.Sprintf("%f", ch.Balance),
			strconv.FormatInt(ch.BalanceUpdatedTime, 10),
			ch.Models,
			ch.Group,
			strconv.FormatInt(ch.UsedQuota, 10),
			modelMapping,
			ratio,
			remark,
			statusCodeMapping,
			priority,
			autoBan,
			ch.OtherInfo,
			tag,
			setting,
			paramOverride,
			headerOverride,
			ch.OtherSettings,
		})
	}
	w.Flush()
}

// -------- Options Export/Import (Root only) --------

func ExportOptionsCSV(c *gin.Context) {
	if !checkRootOnly(c) {
		return
	}
	var options []model.Option
	if err := model.DB.Find(&options).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment;filename=options.csv")
	w := csv.NewWriter(c.Writer)
	w.Write([]string{"key", "value"})
	for _, opt := range options {
		w.Write([]string{opt.Key, opt.Value})
	}
	w.Flush()
}

func ImportOptionsCSV(c *gin.Context) {
	if !checkRootOnly(c) {
		return
	}
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer file.Close()
	r := csv.NewReader(file)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	idx := func(name string) int {
		for i, h := range header {
			if strings.EqualFold(h, name) {
				return i
			}
		}
		return -1
	}
	count := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		kIdx := idx("key")
		vIdx := idx("value")
		if kIdx < 0 || vIdx < 0 {
			continue
		}
		key := strings.TrimSpace(record[kIdx])
		value := record[vIdx]
		if key == "" {
			continue
		}
		var exists model.Option
		if model.DB.Unscoped().First(&exists, "key = ?", key).Error == nil && exists.Key != "" {
			// skip if exists
			continue
		}
		opt := model.Option{
			Key:   key,
			Value: value,
		}
		if err := model.DB.Create(&opt).Error; err == nil {
			count++
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("导入配置完成，成功 %d 条", count),
	})
}

// -------- Import --------

func ImportUsersCSV(c *gin.Context) {
	if !checkRootOnly(c) {
		return
	}
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer file.Close()
	r := csv.NewReader(file)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	idx := func(name string) int {
		for i, h := range header {
			if strings.EqualFold(h, name) {
				return i
			}
		}
		return -1
	}
	count := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		idIdx := idx("id")
		if idIdx < 0 {
			continue
		}
		id := common.String2Int(record[idIdx])
		var exists model.User
		if id > 0 && model.DB.Unscoped().First(&exists, "id = ?", id).Error == nil && exists.Id > 0 {
			continue
		}
		u := model.User{
			Id:              id,
			Username:        record[idx("username")],
			Password:        record[idx("password")],
			DisplayName:     record[idx("display_name")],
			Role:            common.String2Int(record[idx("role")]),
			Status:          common.String2Int(record[idx("status")]),
			Email:           record[idx("email")],
			GitHubId:        record[idx("github_id")],
			DiscordId:       record[idx("discord_id")],
			OidcId:          record[idx("oidc_id")],
			WeChatId:        record[idx("wechat_id")],
			TelegramId:      record[idx("telegram_id")],
			Group:           record[idx("group")],
			Quota:           common.String2Int(record[idx("quota")]),
			UsedQuota:       common.String2Int(record[idx("used_quota")]),
			RequestCount:    common.String2Int(record[idx("request_count")]),
			AffCode:         record[idx("aff_code")],
			AffCount:        common.String2Int(record[idx("aff_count")]),
			AffQuota:        common.String2Int(record[idx("aff_quota")]),
			AffHistoryQuota: common.String2Int(record[idx("aff_history_quota")]),
			InviterId:       common.String2Int(record[idx("inviter_id")]),
			LinuxDOId:       record[idx("linux_do_id")],
			Setting:         record[idx("setting")],
			StripeCustomer:  record[idx("stripe_customer")],
			ToioRegistered:  common.String2Int(record[idx("toio_registered")]),
		}
		// Insert; if id set, GORM will use it unless conflicts; skip if conflict handled above
		if err := model.DB.Create(&u).Error; err == nil {
			count++
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("导入用户完成，成功 %d 条", count),
	})
}

func ImportTokensCSV(c *gin.Context) {
	if !checkRootOnly(c) {
		return
	}
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer file.Close()
	r := csv.NewReader(file)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	idx := func(name string) int {
		for i, h := range header {
			if strings.EqualFold(h, name) {
				return i
			}
		}
		return -1
	}
	count := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		id := common.String2Int(record[idx("id")])
		var exists model.Token
		if id > 0 && model.DB.Unscoped().First(&exists, "id = ?", id).Error == nil && exists.Id > 0 {
			continue
		}
		allowIps := record[idx("allow_ips")]
		var allowIpsPtr *string
		if strings.TrimSpace(allowIps) != "" {
			allowIpsPtr = &allowIps
		}
		channelRulesHighPriority := false
		if column := idx("channel_rules_high_priority"); column >= 0 && column < len(record) {
			channelRulesHighPriority = record[column] == "true"
		}
		t := model.Token{
			Id:                       id,
			UserId:                   common.String2Int(record[idx("user_id")]),
			Key:                      record[idx("key")],
			Status:                   common.String2Int(record[idx("status")]),
			Name:                     record[idx("name")],
			CreatedTime:              int64(common.String2Int(record[idx("created_time")])),
			AccessedTime:             int64(common.String2Int(record[idx("accessed_time")])),
			ExpiredTime:              int64(common.String2Int(record[idx("expired_time")])),
			RemainQuota:              common.String2Int(record[idx("remain_quota")]),
			UnlimitedQuota:           record[idx("unlimited_quota")] == "true",
			ModelLimitsEnabled:       record[idx("model_limits_enabled")] == "true",
			ModelLimits:              record[idx("model_limits")],
			AllowIps:                 allowIpsPtr,
			UsedQuota:                common.String2Int(record[idx("used_quota")]),
			ChannelRules:             record[idx("channel_rules")],
			ChannelRatios:            record[idx("channel_ratios")],
			Group:                    record[idx("group")],
			CrossGroupRetry:          record[idx("cross_group_retry")] == "true",
			ChannelRulesHighPriority: channelRulesHighPriority,
		}
		if err := model.DB.Create(&t).Error; err == nil {
			count++
		} else {
			fmt.Printf("error %#v\n", err)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("导入令牌完成，成功 %d 条", count),
	})
}

func ImportChannelsCSV(c *gin.Context) {
	if !checkRootOnly(c) {
		return
	}
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer file.Close()
	r := csv.NewReader(file)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	idx := func(name string) int {
		for i, h := range header {
			if strings.EqualFold(h, name) {
				return i
			}
		}
		return -1
	}
	count := 0
	channels := make([]model.Channel, 0, 4096)
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		id := common.String2Int(record[idx("id")])
		var exists model.Channel
		if id > 0 && model.DB.Unscoped().First(&exists, "id = ?", id).Error == nil && exists.Id > 0 {
			continue
		}
		weightStr := record[idx("weight")]
		var weightPtr *uint
		wv := uint(common.String2Int(weightStr))
		weightPtr = &wv
		baseURL := record[idx("base_url")]
		var baseURLPtr *string
		baseURLPtr = &baseURL
		modelMapping := record[idx("model_mapping")]
		var modelMappingPtr *string
		modelMappingPtr = &modelMapping
		ratioStr := record[idx("ratio")]
		var ratioPtr *float64
		if strings.TrimSpace(ratioStr) != "" {
			if f, err := strconv.ParseFloat(ratioStr, 64); err == nil {
				ratioPtr = &f
			}
		} else {
			tmp := 0.0
			ratioPtr = &tmp
		}
		remark := record[idx("remark")]
		var remarkPtr *string
		remarkPtr = &remark
		statusCodeMapping := record[idx("status_code_mapping")]
		var statusCodeMappingPtr *string
		statusCodeMappingPtr = &statusCodeMapping
		priorityStr := record[idx("priority")]
		var priorityPtr *int64
		pv := int64(common.String2Int(priorityStr))
		priorityPtr = &pv
		autoBanStr := record[idx("auto_ban")]
		av := common.String2Int(autoBanStr)
		autoBanPtr := &av
		tag := record[idx("tag")]
		tagPtr := &tag
		setting := record[idx("setting")]
		var settingPtr *string
		settingPtr = &setting
		paramOverride := record[idx("param_override")]
		var paramOverridePtr *string
		paramOverridePtr = &paramOverride
		headerOverride := record[idx("header_override")]
		var headerOverridePtr *string
		headerOverridePtr = &headerOverride

		ch := model.Channel{
			Id:                 id,
			Type:               common.String2Int(record[idx("type")]),
			Key:                record[idx("key")],
			Name:               record[idx("name")],
			Weight:             weightPtr,
			CreatedTime:        int64(common.String2Int(record[idx("created_time")])),
			BaseURL:            baseURLPtr,
			Other:              record[idx("other")],
			Status:             common.String2Int(record[idx("status")]),
			Balance:            func() float64 { f, _ := strconv.ParseFloat(record[idx("balance")], 64); return f }(),
			BalanceUpdatedTime: int64(common.String2Int(record[idx("balance_updated_time")])),
			Models:             record[idx("models")],
			Group:              record[idx("group")],
			UsedQuota:          int64(common.String2Int(record[idx("used_quota")])),
			ModelMapping:       modelMappingPtr,
			Ratio:              ratioPtr,
			Remark:             remarkPtr,
			StatusCodeMapping:  statusCodeMappingPtr,
			Priority:           priorityPtr,
			AutoBan:            autoBanPtr,
			OtherInfo:          record[idx("other_info")],
			Tag:                tagPtr,
			Setting:            settingPtr,
			ParamOverride:      paramOverridePtr,
			HeaderOverride:     headerOverridePtr,
			OtherSettings:      record[idx("settings")],
		}
		// 复用现有的校验
		if err := validateChannel(&ch, false); err != nil {
			continue
		}
		channels = append(channels, ch)
		count++
	}
	if len(channels) > 0 {
		if err := model.BatchInsertChannels(channels); err != nil {
			common.ApiError(c, err)
			return
		}
		service.ResetProxyClientCache()
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("导入渠道完成，成功 %d 条", count),
	})
}
