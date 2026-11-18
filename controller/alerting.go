package controller

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/alerting"
	"github.com/QuantumNous/new-api/service/authz"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type alertRuleInput struct {
	ReplayStart int64 `json:"replay_start"`
	ReplayEnd   int64 `json:"replay_end"`
	ReplaySteps int   `json:"replay_steps"`
	RuleID      int64 `json:"rule_id"`
	model.AlertRuleSpec
	Config           *model.AlertRuleSpec `json:"config,omitempty"`
	OwnerUserID      *int                 `json:"owner_user_id,omitempty"`
	ExpectedRevision int64                `json:"expected_revision"`
}

func alertActor(c *gin.Context) alerting.Actor {
	id, role := c.GetInt("id"), c.GetInt("role")
	// Personal alert PAT scopes delegate only the owner's domain, even when
	// the token belongs to Root. Platform operations require a browser session.
	if c.GetBool("use_access_token") {
		return alerting.Actor{ID: id, Role: common.RoleCommonUser}
	}
	return alerting.Actor{ID: id, Role: role, ReadAll: authz.Can(id, role, authz.AlertReadAll)}
}

func alertError(c *gin.Context, err error) {
	status, data := alertErrorResponse(err)
	var cooldown *alertTargetTestCooldownError
	if errors.As(err, &cooldown) {
		c.Header("Retry-After", strconv.FormatInt(cooldown.RetryAfterSeconds, 10))
	}
	if status == http.StatusForbidden || status == http.StatusNotFound {
		objectID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		_ = model.DB.WithContext(c.Request.Context()).Create(&model.AlertAudit{ActorID: c.GetInt("id"), OwnerUserID: c.GetInt("id"), Operation: "access.denied", ObjectID: objectID, CreatedAt: time.Now().UnixMilli()}).Error
	}
	c.JSON(status, data)
}

// Batch imports use the same public errors as individual requests. Never expose
// raw database or destination errors, which can contain notification secrets.
func alertErrorResponse(err error) (int, map[string]any) {
	status, message := http.StatusInternalServerError, "Alert operation failed"
	var blocked *alerting.BlockedAction
	var targetValidation *alerting.TargetValidationError
	var ruleTargetValidation *alerting.RuleTargetValidationError
	var cooldown *alertTargetTestCooldownError
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		status, message = http.StatusNotFound, "Alert resource not found"
	case errors.As(err, &ruleTargetValidation):
		status, message = http.StatusForbidden, ruleTargetValidation.Error()
		if ruleTargetValidation.Disabled {
			status = http.StatusBadRequest
		}
	case errors.Is(err, alerting.ErrForbidden):
		status, message = http.StatusForbidden, "Alert access denied"
	case errors.As(err, &cooldown):
		status, message = http.StatusTooManyRequests, cooldown.Error()
	case errors.Is(err, model.ErrAlertConflict):
		status, message = http.StatusConflict, "Alert configuration changed. Please reload."
	case errors.As(err, &targetValidation):
		status, message = http.StatusBadRequest, targetValidation.MessageKey
	case errors.Is(err, alerting.ErrRuleDeliveryRequired):
		status, message = http.StatusBadRequest, "Select at least one notification target or automatic action."
	case errors.Is(err, alerting.ErrInvalid), errors.Is(err, alerting.ErrTargetRejected):
		status, message = http.StatusBadRequest, "Invalid alert configuration"
	case errors.Is(err, alerting.ErrTargetKeyUnavailable):
		status, message = http.StatusServiceUnavailable, "Notification encryption key is unavailable"
	case errors.As(err, &blocked):
		status, message = http.StatusConflict, "Alert operation is blocked"
		switch blocked.Code {
		case "migration_not_ready":
			message = "Migration requires notification mode, enabled statistics and an active destination."
		case "migration_destination_missing":
			message = "Configure a legacy notification destination before importing."
		case "data_incomplete":
			message = "Usage input incomplete"
		case "migration_jobs_pending":
			message = "Pending deliveries must drain before rollback."
		case "migration_rule_changed":
			message = "The migrated rule is incompatible with the legacy executor."
		case "legacy_report_pending":
			message = "Deliver the pending legacy daily report before cutover."
		case "migration_window_uncovered":
			message = "Wait for complete collection of the current natural day before cutover."
		}
	}
	// Match the shared console error contract so the frontend can translate
	// and display the actual rejection instead of a generic HTTP 400 error.
	text := common.NewMessage(message)
	data := text.Fields()
	data["success"], data["message"] = false, text.Error()
	if targetValidation != nil {
		data["field"] = targetValidation.Field
	}
	if ruleTargetValidation != nil {
		data["field"] = ruleTargetValidation.Field
	}
	if errors.Is(err, alerting.ErrRuleDeliveryRequired) {
		data["field"] = "notify.target_ids"
	}
	if blocked != nil {
		data["reason"] = blocked.Code
	}
	return status, data
}

func alertOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": data})
}

func alertPathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		alertError(c, alerting.ErrInvalid)
		return 0, false
	}
	return id, true
}

func alertOwnerFilter(c *gin.Context, query *gorm.DB) (*gorm.DB, error) {
	query = query.WithContext(c.Request.Context())
	if raw := c.Query("before_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return nil, alerting.ErrInvalid
		}
		query = query.Where("id < ?", id)
	}
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			return nil, alerting.ErrInvalid
		}
	}
	if len(c.Query("q")) > 160 {
		return nil, alerting.ErrInvalid
	}
	return alertVisibleOwnerQuery(c, query)
}

func alertVisibleOwnerQuery(c *gin.Context, query *gorm.DB) (*gorm.DB, error) {
	actor := alertActor(c)
	if c.Query("scope") == "all" {
		if !actor.ReadAll && !actor.Root() {
			return nil, alerting.ErrForbidden
		}
		return query, nil
	}
	owner := actor.ID
	if raw := c.Query("owner_user_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id < 0 {
			return nil, alerting.ErrInvalid
		}
		if id != actor.ID && !actor.Root() && !actor.ReadAll {
			return nil, alerting.ErrForbidden
		}
		owner = id
	}
	return query.Where("owner_user_id = ?", owner), nil
}

// Object links resolve visible current rules with the same ownership boundary
// as the normal list. No dialect-specific JSON query is used for authorization.
func alertRelatedRules(c *gin.Context) ([]int64, bool, error) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	channelText, tokenText := c.Query("channel_id"), c.Query("token_id")
	if channelText == "" && tokenText == "" {
		return nil, false, nil
	}
	channel, token := 0, 0
	var err error
	if channelText != "" {
		channel, err = strconv.Atoi(channelText)
		if err != nil || channel <= 0 || tokenText != "" {
			return nil, true, alerting.ErrInvalid
		}
	}
	owner := 0
	if tokenText != "" {
		token, err = strconv.Atoi(tokenText)
		if err != nil || token <= 0 {
			return nil, true, alerting.ErrInvalid
		}
		var key model.Token
		if err := model.DB.WithContext(ctx).Select("id", "user_id").First(&key, token).Error; err != nil {
			return nil, true, err
		}
		if !alerting.CanRead(alertActor(c), key.UserId) {
			return nil, true, gorm.ErrRecordNotFound
		}
		owner = key.UserId
	}
	query, err := alertVisibleOwnerQuery(c, model.DB.WithContext(ctx).Model(&model.AlertRule{}))
	if err != nil {
		return nil, true, err
	}
	var rules []model.AlertRule
	if err := query.Order("id desc").Limit(1001).Find(&rules).Error; err != nil {
		return nil, true, err
	}
	if len(rules) > 1000 {
		return nil, true, &alerting.BlockedAction{Code: "rule_query_capacity"}
	}
	ids := make([]int64, 0, len(rules))
	for _, rule := range rules {
		spec, err := model.AlertRuleConfig(rule)
		if err != nil {
			continue
		}
		channelScoped := spec.Scope.Type == "channel" || spec.Scope.Type == "channel_model" || spec.Scope.Type == "system" && spec.Condition.Metric == "legacy_notice" && (spec.Condition.LegacySignal == "channel_arrears" || spec.Condition.LegacySignal == "slow_error")
		if channel > 0 && channelScoped && (slices.Contains(spec.Scope.ChannelIDs, channel) || slices.Contains(spec.Scope.EntityIDs, channel)) {
			ids = append(ids, rule.ID)
		}
		if token > 0 && rule.OwnerUserID == owner && (spec.Scope.Type == "account" || spec.Scope.Type == "api_key" && (slices.Contains(spec.Scope.EntityIDs, token) || spec.Scope.Selector == "all_owned" && spec.Scope.IncludeFuture)) {
			ids = append(ids, rule.ID)
		}
	}
	return ids, true, nil
}

func alertPageSize(c *gin.Context, fallback int) int {
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil && n >= 1 && n <= 200 {
			return n
		}
	}
	return fallback
}

func AlertCatalog(c *gin.Context) {
	actor := alertActor(c)
	owner := actor.ID
	if raw := c.Query("owner_user_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			alertError(c, alerting.ErrInvalid)
			return
		}
		if id != actor.ID && !actor.Root() {
			alertError(c, alerting.ErrForbidden)
			return
		}
		owner = id
	}
	scopes := []string{"account", "api_key"}
	if actor.Root() {
		scopes = append(scopes, "channel", "channel_model", "model", "system", "uid_budget", "project_budget")
	}
	budgetOptions, err := alerting.BudgetOptions(c.Request.Context(), actor)
	if err != nil {
		alertError(c, err)
		return
	}
	var keys []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := model.DB.WithContext(c.Request.Context()).Model(&model.Token{}).Select("id", "name").Where("user_id = ?", owner).Order("id desc").Limit(500).Scan(&keys).Error; err != nil {
		alertError(c, err)
		return
	}
	var subscriptions []model.UserSubscription
	if err := model.DB.WithContext(c.Request.Context()).Select("id", "plan_id", "start_time", "end_time").Where("user_id = ? AND status = ? AND end_time > ? AND amount_total > 0", owner, "active", time.Now().Unix()).Limit(100).Find(&subscriptions).Error; err != nil {
		alertError(c, err)
		return
	}
	planIDs := make([]int, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		planIDs = append(planIDs, subscription.PlanId)
	}
	var plans []model.SubscriptionPlan
	if len(planIDs) > 0 {
		if err := model.DB.WithContext(c.Request.Context()).Select("id", "title").Where("id IN ?", planIDs).Find(&plans).Error; err != nil {
			alertError(c, err)
			return
		}
	}
	planNames := map[int]string{}
	for _, plan := range plans {
		planNames[plan.Id] = plan.Title
	}
	subscriptionOptions := make([]map[string]any, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		subscriptionOptions = append(subscriptionOptions, map[string]any{"id": subscription.Id, "name": planNames[subscription.PlanId], "start_time": subscription.StartTime, "end_time": subscription.EndTime})
	}
	alertOK(c, gin.H{"subscriptions": subscriptionOptions, "scopes": scopes, "metrics": slices.DeleteFunc(slices.Clone(alerting.Metrics), func(metric string) bool { return metric == "legacy_notice" }), "target_types": []string{"email", "feishu", "wecom", "webhook_v1", "webhook_v2", "bark", "gotify"}, "aggregations": []string{"mean", "p95", "p99"}, "deadline_kinds": []string{"first_output", "output_idle", "total"}, "enabled": common.AlertingEnabled, "actions_enabled": common.AlertingActionsEnabled, "encryption_configured": service.AlertTargetVault != nil, "read_all": actor.ReadAll || actor.Root(), "manage_platform": actor.Root(), "read_engine_health": authz.Can(actor.ID, actor.Role, authz.AlertEngineRead), "api_keys": keys, "budget_options": budgetOptions, "shadow_mode": common.AlertingShadowMode})
}

// alertRuleList applies configuration filters before pagination. Config is TEXT
// on every supported database; scanning only IDs/configs in bounded batches
// avoids dialect-specific JSON operators and tolerates a corrupt legacy rule.
func alertRuleList(c *gin.Context, query *gorm.DB) ([]model.AlertRule, int64, int, error) {
	page, limit := 0, alertPageSize(c, 200)
	if raw, paged := c.GetQuery("page"); paged {
		var err error
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 || page > 1000000 || c.Query("before_id") != "" {
			return nil, 0, 0, alerting.ErrInvalid
		}
		limit = alertPageSize(c, 50)
	}
	status := c.Query("status")
	if status != "" && status != "enabled" && status != "paused" {
		return nil, 0, 0, alerting.ErrInvalid
	}
	if status != "" {
		query = query.Where("enabled = ?", status == "enabled")
	}
	if search := strings.TrimSpace(c.Query("q")); search != "" {
		pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(search)) + "%"
		id, _ := strconv.ParseInt(search, 10, 64)
		query = query.Where("LOWER(name) LIKE ? ESCAPE '!' OR id = ?", pattern, id)
	}
	scope, metric := c.Query("scope_type"), c.Query("metric")
	severity, mode := c.Query("severity"), c.Query("management_mode")
	for _, filter := range []struct {
		value   string
		allowed []string
	}{
		{scope, []string{"account", "api_key", "channel", "channel_model", "model", "system", "uid_budget", "project_budget"}},
		{metric, alerting.Metrics},
		{severity, []string{"P0", "P1", "P2", "P3"}},
		{mode, []string{"user_managed", "platform_enforced"}},
	} {
		if filter.value != "" && !slices.Contains(filter.allowed, filter.value) {
			return nil, 0, 0, alerting.ErrInvalid
		}
	}
	var total int64
	if scope != "" || metric != "" || severity != "" || mode != "" {
		matching := make([]int64, 0)
		var before int64
		for {
			batch := query.Session(&gorm.Session{}).Select("id", "config").Order("id desc").Limit(200)
			if before > 0 {
				batch = batch.Where("id < ?", before)
			}
			var candidates []model.AlertRule
			if err := batch.Find(&candidates).Error; err != nil {
				return nil, 0, 0, err
			}
			for _, row := range candidates {
				spec, err := model.AlertRuleConfig(row)
				if err != nil {
					continue
				}
				if (scope == "" || spec.Scope.Type == scope) && (metric == "" || spec.Condition.Metric == metric) && (severity == "" || spec.Severity == severity) && (mode == "" || spec.ManagementMode == mode) {
					matching = append(matching, row.ID)
				}
			}
			if len(candidates) < 200 {
				break
			}
			before = candidates[len(candidates)-1].ID
		}
		total = int64(len(matching))
		if page > 0 {
			page = min(page, max(1, (len(matching)+limit-1)/limit))
		}
		start := 0
		if page > 0 {
			start = (page - 1) * limit
		}
		query = query.Where("id IN ?", matching[start:min(start+limit, len(matching))])
	} else if page > 0 {
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return nil, 0, 0, err
		}
		page = min(page, max(1, int((total+int64(limit)-1)/int64(limit))))
		query = query.Offset((page - 1) * limit)
	}
	rows := make([]model.AlertRule, 0, limit)
	err := query.Order("id desc").Limit(limit).Find(&rows).Error
	return rows, total, page, err
}

func ListAlertRules(c *gin.Context) {
	query, err := alertOwnerFilter(c, model.DB.WithContext(c.Request.Context()).Model(&model.AlertRule{}))
	if err != nil {
		alertError(c, err)
		return
	}
	ids, filtered, err := alertRelatedRules(c)
	if err != nil {
		alertError(c, err)
		return
	}
	if filtered {
		query = query.Where("id IN ?", ids)
	}
	rows, total, page, err := alertRuleList(c, query)
	if err != nil {
		alertError(c, err)
		return
	}
	var ruleIDs []int64
	for _, rule := range rows {
		ruleIDs = append(ruleIDs, rule.ID)
	}
	var groups []struct {
		RuleID     int64
		Generation int64
		DataStatus string
		Count      int64
		UpdatedAt  int64
	}
	if len(ruleIDs) > 0 {
		if err := model.DB.WithContext(c.Request.Context()).Model(&model.AlertRuleState{}).Select("rule_id,generation,data_status,COUNT(*) AS count,MAX(updated_at) AS updated_at").Where("rule_id IN ?", ruleIDs).Group("rule_id,generation,data_status").Scan(&groups).Error; err != nil {
			alertError(c, err)
			return
		}
	}
	data := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		response, err := alerting.RuleResponse(row)
		if err != nil {
			alertError(c, err)
			return
		}
		priority := map[string]int{"complete": 0, "warming_up": 1, "no_data": 2, "unknown": 3, "partial": 4, "stale": 5, "error": 6}
		status, total, updated := "no_data", int64(0), int64(0)
		for _, group := range groups {
			if group.RuleID != row.ID || group.Generation != row.Generation {
				continue
			}
			if total == 0 || priority[group.DataStatus] > priority[status] {
				status = group.DataStatus
			}
			total += group.Count
			updated = max(updated, group.UpdatedAt)
		}
		if updated > 0 && time.Now().UnixMilli()-updated > 60000 && row.Enabled {
			status = "stale"
		}
		if row.LastEvaluationError != "" {
			status = "error"
		}
		response["last_evaluation_error"] = row.LastEvaluationError
		response["runtime"] = gin.H{"data_status": status, "target_count": total, "updated_at": updated}
		data = append(data, response)
	}
	if page > 0 {
		alertOK(c, gin.H{"items": data, "total": total, "page": page, "page_size": alertPageSize(c, 50)})
		return
	}
	alertOK(c, data)
}

func GetAlertRule(c *gin.Context) {
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	row, err := alerting.VisibleRule(c.Request.Context(), alertActor(c), id, false)
	if err != nil {
		alertError(c, err)
		return
	}
	response, err := alerting.RuleResponse(row)
	if err != nil {
		alertError(c, err)
		return
	}
	var states []model.AlertRuleState
	if err := model.DB.Where("rule_id = ? AND generation = ?", row.ID, row.Generation).Limit(500).Find(&states).Error; err != nil {
		alertError(c, err)
		return
	}
	response["states"] = states
	spec, err := model.AlertRuleConfig(row)
	if err != nil {
		alertOK(c, response)
		return
	}
	targets, err := alerting.ResolveTargets(c.Request.Context(), row, spec)
	if err != nil {
		alertError(c, err)
		return
	}
	response["targets"] = targets
	alertOK(c, response)
}

func SaveAlertRule(c *gin.Context) {
	var input alertRuleInput
	if c.ShouldBindJSON(&input) != nil {
		alertError(c, alerting.ErrInvalid)
		return
	}
	if input.Config != nil {
		input.AlertRuleSpec = *input.Config
	}
	actor := alertActor(c)
	owner := actor.ID
	if input.OwnerUserID != nil {
		owner = *input.OwnerUserID
	}
	if alerting.PlatformScope(input.Scope.Type) && input.OwnerUserID == nil {
		owner = 0
	}
	if err := alerting.Validate(c.Request.Context(), actor, owner, &input.AlertRuleSpec); err != nil {
		alertError(c, err)
		return
	}
	var id int64
	if c.Param("id") != "" {
		var ok bool
		id, ok = alertPathID(c)
		if !ok {
			return
		}
	}
	proofContext := service.AlertManageContext{RuleID: id, OwnerUserID: owner, ExpectedRevision: input.ExpectedRevision, ConfigHash: model.AlertIdentity(input.AlertRuleSpec), Operation: "save"}
	if input.Enabled && len(input.Actions) > 0 {
		encoded, err := common.Marshal(proofContext)
		if err != nil {
			alertError(c, err)
			return
		}
		if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeAlertManage, Context: encoded}) == nil {
			return
		}
	}
	row, err := alerting.SaveRule(c.Request.Context(), actor, owner, id, input.ExpectedRevision, input.AlertRuleSpec)
	if err != nil {
		alertError(c, err)
		return
	}
	response, err := alerting.RuleResponse(*row)
	if err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, response)
}

func PreviewAlertRule(c *gin.Context) {
	var input alertRuleInput
	if c.ShouldBindJSON(&input) != nil {
		alertError(c, alerting.ErrInvalid)
		return
	}
	if input.Config != nil {
		input.AlertRuleSpec = *input.Config
	}
	actor := alertActor(c)
	owner := actor.ID
	if input.OwnerUserID != nil {
		owner = *input.OwnerUserID
	}
	if alerting.PlatformScope(input.Scope.Type) {
		owner = 0
	}
	if err := alerting.Validate(c.Request.Context(), actor, owner, &input.AlertRuleSpec); err != nil {
		alertError(c, err)
		return
	}
	if input.RuleID > 0 {
		if _, err := alerting.VisibleRule(c.Request.Context(), actor, input.RuleID, true); err != nil {
			alertError(c, err)
			return
		}
	}
	now := time.Now()
	// Preview reads available history; a synthetic activation time must not
	// shorten a maximum-length window behind the accepted source watermark.
	row := model.AlertRule{OwnerUserID: owner, EffectiveAt: now.Add(-24 * time.Hour).UnixMilli()}
	if input.Condition.Window.Type == "rolling" || input.Trigger.Mode == "schedule_report" {
		row.EffectiveAt = 0
	}
	targets, err := alerting.ResolveTargets(c.Request.Context(), row, input.AlertRuleSpec)
	if err != nil {
		alertError(c, err)
		return
	}
	if strings.HasSuffix(c.FullPath(), "/backtest") {
		start, end, steps := input.ReplayStart, input.ReplayEnd, input.ReplaySteps
		if end == 0 {
			end = now.UnixMilli()
		}
		if start == 0 {
			start = now.Add(-time.Hour).UnixMilli()
		}
		if steps == 0 {
			steps = 12
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		points, err := alerting.ReplayRule(ctx, row, input.AlertRuleSpec, start, end, steps, now)
		if err != nil {
			alertError(c, err)
			return
		}
		alertOK(c, points)
		return
	}
	data := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		m, err := alerting.Measure(c.Request.Context(), row, input.AlertRuleSpec, target, now)
		if err != nil {
			alertError(c, err)
			return
		}
		data = append(data, map[string]any{"target": target, "measurement": m})
		if len(input.Actions) > 0 && (target.Type == "channel" || target.Type == "channel_model") {
			impact, err := alerting.ChannelActionImpact(model.DB.WithContext(c.Request.Context()), target, input.AlertRuleSpec)
			if err != nil {
				alertError(c, err)
				return
			}
			data[len(data)-1]["impact"] = impact
		}
	}
	if input.RuleID > 0 {
		if _, err := alerting.VisibleRule(c.Request.Context(), actor, input.RuleID, true); err != nil {
			alertError(c, err)
			return
		}
	}
	alertOK(c, gin.H{"targets": data, "config": input.AlertRuleSpec, "verification_context": service.AlertManageContext{RuleID: input.RuleID, OwnerUserID: owner, ExpectedRevision: input.ExpectedRevision, ConfigHash: model.AlertIdentity(input.AlertRuleSpec), Operation: "save"}})
}

func SetAlertRulePaused(c *gin.Context) {
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	actor := alertActor(c)
	row, err := alerting.VisibleRule(c.Request.Context(), actor, id, true)
	if err != nil {
		alertError(c, err)
		return
	}
	var input struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if c.ShouldBindJSON(&input) != nil || input.ExpectedRevision != row.Revision {
		alertError(c, model.ErrAlertConflict)
		return
	}
	spec, err := model.AlertRuleConfig(row)
	if err != nil {
		alertError(c, err)
		return
	}
	spec.Enabled = strings.HasSuffix(c.FullPath(), "/resume")
	if spec.Enabled && len(spec.Actions) > 0 {
		encoded, _ := common.Marshal(service.AlertManageContext{RuleID: id, OwnerUserID: row.OwnerUserID, ExpectedRevision: row.Revision, ConfigHash: model.AlertIdentity(spec), Operation: "save"})
		if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeAlertManage, Context: encoded}) == nil {
			return
		}
	}
	saved, err := alerting.SaveRule(c.Request.Context(), actor, row.OwnerUserID, id, row.Revision, spec)
	if err != nil {
		alertError(c, err)
		return
	}
	response, _ := alerting.RuleResponse(*saved)
	alertOK(c, response)
}

func DeleteAlertRule(c *gin.Context) {
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	row, err := alerting.VisibleRule(c.Request.Context(), alertActor(c), id, true)
	if err != nil {
		alertError(c, err)
		return
	}
	var input struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if c.ShouldBindJSON(&input) != nil || row.Revision != input.ExpectedRevision {
		alertError(c, model.ErrAlertConflict)
		return
	}
	err = model.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ? AND revision = ?", id, row.Revision).Delete(&model.AlertRule{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return model.ErrAlertConflict
		}
		return tx.Create(&model.AlertAudit{ActorID: alertActor(c).ID, OwnerUserID: row.OwnerUserID, Operation: "rule.delete", ObjectID: id, Revision: row.Revision, CreatedAt: time.Now().UnixMilli()}).Error
	})
	if err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, nil)
}

func ListAlertTargets(c *gin.Context) {
	query, err := alertOwnerFilter(c, model.DB.WithContext(c.Request.Context()).Model(&model.AlertTarget{}))
	if err != nil {
		alertError(c, err)
		return
	}
	if search := strings.TrimSpace(c.Query("q")); search != "" {
		if id, err := strconv.ParseInt(search, 10, 64); err == nil && id > 0 {
			query = query.Where("name LIKE ? OR id = ?", "%"+search+"%", id)
		} else {
			query = query.Where("name LIKE ?", "%"+search+"%")
		}
	}
	if raw := c.Query("ids"); raw != "" {
		var ids []int64
		for part := range strings.SplitSeq(raw, ",") {
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil || id <= 0 || len(ids) >= 20 {
				alertError(c, alerting.ErrInvalid)
				return
			}
			ids = append(ids, id)
		}
		query = query.Where("id IN ?", ids)
	}
	var rows []model.AlertTarget
	if err = query.Order("id desc").Limit(alertPageSize(c, 200)).Find(&rows).Error; err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, rows)
}

func SaveAlertTarget(c *gin.Context) {
	var input alerting.TargetInput
	if c.ShouldBindJSON(&input) != nil {
		alertError(c, alerting.ErrInvalid)
		return
	}
	var id int64
	if c.Param("id") != "" {
		var ok bool
		id, ok = alertPathID(c)
		if !ok {
			return
		}
	}
	actor := alertActor(c)
	owner := actor.ID
	if raw := c.Query("owner_user_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id < 0 {
			alertError(c, alerting.ErrInvalid)
			return
		}
		if id != actor.ID && !actor.Root() {
			alertError(c, alerting.ErrForbidden)
			return
		}
		owner = id
	}
	if service.AlertTargetVault == nil {
		alertError(c, alerting.ErrTargetKeyUnavailable)
		return
	}
	row, err := service.AlertTargetVault.Save(c.Request.Context(), actor, owner, id, input)
	if err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, row)
}

func DeleteAlertTarget(c *gin.Context) {
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	actor := alertActor(c)
	var row model.AlertTarget
	if err := model.DB.First(&row, id).Error; err != nil {
		alertError(c, err)
		return
	}
	if row.OwnerUserID != actor.ID && !actor.Root() {
		alertError(c, alerting.ErrForbidden)
		return
	}
	var input struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if c.ShouldBindJSON(&input) != nil || input.ExpectedRevision != row.Revision {
		alertError(c, model.ErrAlertConflict)
		return
	}
	err := model.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ? AND revision = ?", id, row.Revision).Delete(&model.AlertTarget{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return model.ErrAlertConflict
		}
		return tx.Create(&model.AlertAudit{ActorID: actor.ID, OwnerUserID: row.OwnerUserID, Operation: "target.delete", ObjectID: id, Revision: row.Revision, CreatedAt: time.Now().UnixMilli()}).Error
	})
	if err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, nil)
}

func ListAlertEvents(c *gin.Context) {
	query, err := alertOwnerFilter(c, model.DB.Model(&model.AlertEvent{}))
	if err != nil {
		alertError(c, err)
		return
	}
	ids, filtered, err := alertRelatedRules(c)
	if err != nil {
		alertError(c, err)
		return
	}
	if filtered {
		if len(ids) == 0 {
			alertOK(c, []any{})
			return
		}
		query = query.Where("rule_id IN ?", ids)
	}
	if search := strings.TrimSpace(c.Query("q")); search != "" {
		query = query.Where("target_name LIKE ?", "%"+search+"%")
	}
	var rows []model.AlertEvent
	if err := query.Order("id desc").Limit(alertPageSize(c, 100)).Find(&rows).Error; err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, rows)
}

func GetAlertEvent(c *gin.Context) {
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	var row model.AlertEvent
	if err := model.DB.First(&row, id).Error; err != nil {
		alertError(c, err)
		return
	}
	if !alerting.CanRead(alertActor(c), row.OwnerUserID) {
		alertError(c, gorm.ErrRecordNotFound)
		return
	}
	var jobs []model.AlertJob
	if err := model.DB.Where("event_id = ?", id).Order("id").Limit(50).Find(&jobs).Error; err != nil {
		alertError(c, err)
		return
	}
	config, err := model.AlertRuleConfig(model.AlertRule{Config: row.RuleConfig})
	if err != nil && row.Kind != "test" && row.Kind != "engine_health" {
		alertError(c, err)
		return
	}
	canManage := alertActor(c).Root() || row.RuleID == 0 && alertActor(c).ID == row.OwnerUserID
	if row.RuleID > 0 {
		if rule, err := alerting.VisibleRule(c.Request.Context(), alertActor(c), row.RuleID, false); err == nil {
			canManage = alerting.CanManage(alertActor(c), rule)
		}
	}
	alertOK(c, gin.H{"event": row, "jobs": jobs, "config": config, "can_manage": canManage})
}

func ListAlertIncidents(c *gin.Context) {
	query, err := alertOwnerFilter(c, model.DB.Model(&model.AlertIncident{}))
	if err != nil {
		alertError(c, err)
		return
	}
	ids, filtered, err := alertRelatedRules(c)
	if err != nil {
		alertError(c, err)
		return
	}
	if filtered {
		if len(ids) == 0 {
			alertOK(c, []any{})
			return
		}
		query = query.Where("rule_id IN ?", ids)
	}
	if search := strings.TrimSpace(c.Query("q")); search != "" {
		query = query.Where("target_name LIKE ?", "%"+search+"%")
	}
	var rows []model.AlertIncident
	if err := query.Order("id desc").Limit(alertPageSize(c, 100)).Find(&rows).Error; err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, rows)
}

func HandleAlertIncident(c *gin.Context) {
	if c.Param("operation") != "acknowledge" && c.Param("operation") != "close" {
		alertError(c, alerting.ErrInvalid)
		return
	}
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	var row model.AlertIncident
	if err := model.DB.First(&row, id).Error; err != nil {
		alertError(c, err)
		return
	}
	actor := alertActor(c)
	rule, err := alerting.VisibleRule(c.Request.Context(), actor, row.RuleID, true)
	if err != nil || !alerting.CanManage(actor, rule) {
		alertError(c, gorm.ErrRecordNotFound)
		return
	}
	updates := map[string]any{"acknowledged_at": time.Now().UnixMilli(), "acknowledged_by": actor.ID, "handling_status": "acknowledged"}
	if c.Param("operation") == "close" {
		var input struct {
			Reason string `json:"reason"`
		}
		if c.ShouldBindJSON(&input) != nil || len(input.Reason) == 0 || len(input.Reason) > 1000 {
			alertError(c, alerting.ErrInvalid)
			return
		}
		updates["closed_at"], updates["closed_by"], updates["close_reason"], updates["handling_status"] = time.Now().UnixMilli(), actor.ID, input.Reason, "closed"
	}
	err = model.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&row).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Create(&model.AlertAudit{ActorID: actor.ID, OwnerUserID: row.OwnerUserID, Operation: "incident." + c.Param("operation"), ObjectID: id, CreatedAt: time.Now().UnixMilli()}).Error
	})
	if err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, nil)
}

func GetAlertEngineHealth(c *gin.Context) {
	actor := alertActor(c)
	if !authz.Can(actor.ID, actor.Role, authz.AlertEngineRead) {
		alertError(c, alerting.ErrForbidden)
		return
	}
	data, err := service.AlertEngineHealth(c.Request.Context())
	if err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, data)
}

func AlertChannelModels(c *gin.Context) {
	actor := alertActor(c)
	if !actor.Root() {
		alertError(c, alerting.ErrForbidden)
		return
	}
	var rows []model.Channel
	if err := model.DB.Select("id", "name", "models", "status").Order("id").Limit(1000).Find(&rows).Error; err != nil {
		alertError(c, err)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var policy model.AlertChannelPolicy
		result := model.DB.WithContext(c.Request.Context()).Where("channel_id = ?", row.Id).Limit(1).Find(&policy)
		if result.Error != nil {
			alertError(c, result.Error)
			return
		}
		mode := policy.ProtectionMode
		if mode == "" {
			mode = "legacy"
		}
		items = append(items, map[string]any{"id": row.Id, "name": row.Name, "models": row.Models, "status": row.Status, "protection_mode": mode, "protection_revision": policy.Revision})
	}
	alertOK(c, items)
}
