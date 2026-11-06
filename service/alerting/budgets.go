package alerting

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/shopspring/decimal"
)

func BudgetOptions(ctx context.Context, actor Actor) ([]map[string]any, error) {
	items := []map[string]any{}
	if !actor.Root() {
		return items, nil
	}
	var uids []model.CliendUserQuota
	if err := model.DB.WithContext(ctx).Select("id", "client_user_id", "client_name").Order("id desc").Limit(500).Find(&uids).Error; err != nil {
		return nil, err
	}
	for _, uid := range uids {
		items = append(items, map[string]any{"type": "uid_budget", "id": uid.Id, "name": uid.ClientName + " / " + uid.ClientUserId})
	}
	var allocations []model.ProjectAllocation
	if err := model.DB.WithContext(ctx).Order("id desc").Limit(500).Find(&allocations).Error; err != nil {
		return nil, err
	}
	for _, allocation := range allocations {
		var project model.Project
		var plan model.ProjectAllocationPlan
		if err := model.DB.WithContext(ctx).Select("id", "project_name").First(&project, allocation.ProjectId).Error; err != nil {
			return nil, err
		}
		if err := model.DB.WithContext(ctx).Select("id", "plan_name").First(&plan, allocation.PlanId).Error; err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"type": "project_budget", "id": allocation.Id, "name": project.ProjectName + " / " + plan.PlanName + " / " + allocation.ClientUserId})
	}
	return items, nil
}

// Budget metrics read existing wallet/subscription and allocation authorities.
// They neither recompute billing nor combine independent UID/project pools.
func measureBudget(ctx context.Context, rule model.AlertRule, spec model.AlertRuleSpec, target MonitoredTarget, now time.Time) (Measurement, error) {
	m := Measurement{Status: "complete", WindowStart: now.UnixMilli(), WindowEnd: now.UnixMilli(), Samples: 1, AttributionCovered: true}
	c := spec.Condition
	if c.Metric == "balance_remaining" {
		if c.BalancePool == "subscription" {
			var subscription model.UserSubscription
			if err := model.DB.WithContext(ctx).Where("id = ? AND user_id = ?", c.SubscriptionID, rule.OwnerUserID).First(&subscription).Error; err != nil {
				return m, err
			}
			if subscription.Status != "active" || subscription.EndTime <= now.Unix() || subscription.AmountTotal == 0 {
				m.Status, m.Reason = "no_data", "subscription_not_finite_active"
				return m, nil
			}
			m.Value = subscription.AmountTotal - subscription.AmountUsed
			return m, nil
		}
		var account model.User
		if err := model.DB.WithContext(ctx).Select("id", "quota").First(&account, rule.OwnerUserID).Error; err != nil {
			return m, err
		}
		m.Value = int64(account.Quota)
		return m, nil
	}
	var budget, used int64
	if target.Type == "uid_budget" && !common.DataExportEnabled {
		m.Status, m.Reason = "partial", "usage_source_disabled"
		return m, nil
	}
	if target.Type == "uid_budget" {
		_, status, reason, err := SourceWatermark(ctx, now)
		if err != nil {
			return m, err
		}
		if status == "complete" {
			status, reason, err = UsageCoverage(ctx, now)
			if err != nil {
				return m, err
			}
		}
		if status != "complete" {
			m.Status, m.Reason = status, reason
			return m, nil
		}
		var row model.CliendUserQuota
		if err := model.DB.WithContext(ctx).First(&row, target.ID).Error; err != nil {
			return m, err
		}
		usd := int64(row.FixedQuota)
		if row.ExpiredAt == 0 || row.ExpiredAt > now.Unix() {
			usd += int64(row.TempQuota)
		}
		budget, err = USDQuota(decimal.NewFromInt(max(usd, 0)).String())
		if err != nil {
			return m, err
		}
		used, err = model.GetMonthlyNonProjectQuota(row.ClientUserId, common.BillingMonthStartUnix(now.Unix()))
		if err != nil {
			return m, err
		}
	} else if target.Type == "project_budget" {
		var allocation model.ProjectAllocation
		if err := model.DB.WithContext(ctx).First(&allocation, target.ID).Error; err != nil {
			return m, err
		}
		var project model.Project
		var plan model.ProjectAllocationPlan
		if err := model.DB.WithContext(ctx).First(&project, allocation.ProjectId).Error; err != nil {
			return m, err
		}
		if err := model.DB.WithContext(ctx).Where("id = ? AND project_id = ?", allocation.PlanId, allocation.ProjectId).First(&plan).Error; err != nil {
			return m, err
		}
		if project.Status != model.ProjectStatusEnabled || project.ActivePlanId != allocation.PlanId || plan.IsExpired() {
			m.Status, m.Reason = "no_data", "allocation_not_active"
			return m, nil
		}
		var err error
		budget, err = USDQuota(decimal.NewFromInt(int64(max(allocation.AllocatedQuota, 0))).String())
		if err != nil {
			return m, err
		}
		used = int64(allocation.UsedQuota)
	} else {
		return m, ErrInvalid
	}
	if budget <= 0 {
		m.Status, m.Reason = "no_data", "budget_not_positive"
		return m, nil
	}
	remaining := decimal.Max(decimal.Zero, decimal.NewFromInt(budget).Sub(decimal.NewFromInt(used)))
	ratio := remaining.Mul(decimal.NewFromInt(10000)).Div(decimal.NewFromInt(budget)).Truncate(0).BigInt()
	if !ratio.IsInt64() {
		return m, ErrInvalid
	}
	m.Value = ratio.Int64()
	return m, nil
}
