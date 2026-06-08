// Package service - org_view.go
//
// 组织标签系统的"能看哪些页 + 看到什么数据"两个核心问题的实现。
// 详细设计见 org.md。
package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"golang.org/x/net/context"
)

// ---------------- 页面 key 常量 ----------------

// 页面 key:用于菜单白名单 + PageAuth 中间件。命名跟前端路由路径对齐(便于追踪),
// 但实际是字符串契约,不强绑路由。
const (
	PageLog                      = "log"
	PageQuotaStatistics          = "quota_statistics"
	PageClientUserQuota          = "client_user_quota"
	PageProject                  = "project"
	PageBill                     = "bill"
	PageSettlementConfigReadonly = "settlement_config_readonly"

	// "普通用户菜单" — 已登录用户默认能看到的页面集合。
	// 这些 key 用于 wl-admin 和 other 各角色的 additive 模式基础集。
	PageToken      = "token"
	PageTopup      = "topup"
	PagePersonal   = "personal"
	PageDetail     = "detail"     // 数据看板
	PageMidjourney = "midjourney" // 绘图日志
	PageTask       = "task"       // 任务日志
	PageErrorLog   = "errorlog"
	PageChat       = "chat"
	PagePlayground = "playground"
	PageBillSelf   = "bill_self" // 普通用户的账单(看自己)

	// 仅管理员可见的页面 — 系统 admin/root 的完整菜单要包含。
	PageChannel             = "channel"
	PageModels              = "models"
	PageDeployment          = "deployment"
	PageModelChannelMonitor = "modelChannelMonitor"
	PageRedemption          = "redemption"
	PageSubscription        = "subscription"
	PageUser                = "user"
	PageModelRouteConfig    = "model_route_config"
	PageSetting             = "setting"
	PageSettlementConfig    = "settlement_config" // 可编辑(root)
)

// TopbarMode 顶栏行为
const (
	TopbarNormal     = "normal"
	TopbarLogoutOnly = "logout_only"
)

// UserMenu 是 GET /api/user/menu 的 response 形态。前端登录后调一次缓存起来,
// SiderBar/Headerbar/路由守卫都消费它,不再自己判断 toio/role 这套。
type UserMenu struct {
	TopbarMode string   `json:"topbar_mode"`
	Pages      []string `json:"pages"`
}

// defaultUserPages 普通用户(role=common)默认菜单。
// 注意:不包含 PageErrorLog(admin 才看)、PageQuotaStatistics(leader+ 才看)——
// 这些通过 pagesForCommonUser 按全局 role 条件加上,与原 newapi 行为保持一致。
var defaultUserPages = []string{
	PageDetail,
	PageToken,
	PageLog,
	PageMidjourney,
	PageTask,
	PageBillSelf,
	PageTopup,
	PagePersonal,
	PageChat,
	PagePlayground,
}

// pagesForCommonUser 为没有特殊 org 加成的用户(other / wl 非 admin)计算菜单。
// 在 defaultUserPages 基础上,按全局 role 条件加 PageQuotaStatistics(leader+)。
// 与原 newapi 行为一致:消耗统计原本就是 leader+ 才能看(SiderBar 里 tableHiddle 隐藏)。
func pagesForCommonUser(role int) []string {
	pages := append([]string{}, defaultUserPages...)
	if role >= common.RoleLeaderUser {
		pages = append(pages, PageQuotaStatistics)
	}
	return pages
}

// allAdminPages 系统 admin 的完整菜单；root-only 页面单独追加。
var allAdminPages = append(append([]string{}, defaultUserPages...),
	PageChannel,
	PageModels,
	PageDeployment,
	PageRedemption,
	PageSubscription,
	PageUser,
	PageClientUserQuota,
	PageProject,
	PageModelRouteConfig,
	PageSetting,
	PageSettlementConfig,
	PageBill,
)

var allRootPages = append(append([]string{}, allAdminPages...), PageModelChannelMonitor)

// GetUserMenu 返回当前用户能看到的菜单 + 顶栏模式。
// 设计核心:权限策略只在这里,一个 switch case,改权限就改这里。
//
// 优先级:
//  1. 系统 root / admin / leader 全局角色 — 看对应完整菜单(跨 org)。
//  2. 按 (org_code, org_role) switch。
//  3. 兜底 "other" 普通用户行为。
func GetUserMenu(user *model.User) UserMenu {
	if user == nil {
		// 未登录:理论上不会进到这里,兜底空菜单
		return UserMenu{TopbarMode: TopbarNormal, Pages: nil}
	}

	// 模型渠道监控只给系统 root 账号看。
	if user.Role >= common.RoleRootUser {
		return UserMenu{TopbarMode: TopbarNormal, Pages: allRootPages}
	}

	// 全局 admin 跨 org,始终看完整菜单，但不包含 root-only 页面。
	if user.Role >= common.RoleAdminUser {
		return UserMenu{TopbarMode: TopbarNormal, Pages: allAdminPages}
	}

	switch user.OrgCode {
	case "mt":
		// mt 砍光模式:顶栏只剩登出,菜单白名单按角色拆
		switch user.OrgRole {
		case constant.OrgRoleMember:
			return UserMenu{TopbarMode: TopbarLogoutOnly, Pages: []string{PageLog}}
		case constant.OrgRoleLeader:
			return UserMenu{TopbarMode: TopbarLogoutOnly, Pages: []string{PageLog, PageQuotaStatistics}}
		case constant.OrgRoleAdmin:
			// mt-admin: 使用日志 + 消耗统计 + UID 预算 + 项目预算(累加包含 leader 的能力)
			return UserMenu{TopbarMode: TopbarLogoutOnly, Pages: []string{PageLog, PageQuotaStatistics, PageClientUserQuota, PageProject}}
		}
		// 未知 mt 角色兜底为 member 最小集合
		return UserMenu{TopbarMode: TopbarLogoutOnly, Pages: []string{PageLog}}

	case "wl":
		// wl 增量模式:普通用户菜单 + wl-admin 额外能力,但全 wl 用户都不显示
		// 操练场(playground)、聊天(chat)、结算价格管理(settlement_config_readonly)
		wlBase := removePages(pagesForCommonUser(user.Role), PageChat, PagePlayground)
		if user.OrgRole == constant.OrgRoleAdmin {
			// wl-admin 额外能力:消耗统计 + 全组织账单查询(去掉结算价格)
			extra := []string{PageQuotaStatistics, PageBill}
			return UserMenu{TopbarMode: TopbarNormal, Pages: mergePages(wlBase, extra)}
		}
		return UserMenu{TopbarMode: TopbarNormal, Pages: wlBase}

	default:
		// "other" / "" / 未匹配 → 普通用户(leader+ 额外看消耗统计)
		return UserMenu{TopbarMode: TopbarNormal, Pages: pagesForCommonUser(user.Role)}
	}
}

// HasPage 判断当前用户能否访问某个 page key。
// 系统 admin / root 始终通过(GetUserMenu 已经给他完整菜单)。
func HasPage(user *model.User, pageKey string) bool {
	menu := GetUserMenu(user)
	for _, p := range menu.Pages {
		if p == pageKey {
			return true
		}
	}
	return false
}

// mergePages 合并多个 page 列表,去重保序。
func mergePages(base []string, extra ...[]string) []string {
	seen := make(map[string]bool, len(base))
	out := make([]string, 0, len(base))
	for _, p := range base {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, list := range extra {
		for _, p := range list {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// removePages 从 base 中剔除指定 page,保留剩余顺序。返回新切片,不修改 base。
func removePages(base []string, drop ...string) []string {
	if len(drop) == 0 {
		return base
	}
	dropSet := make(map[string]bool, len(drop))
	for _, p := range drop {
		dropSet[p] = true
	}
	out := make([]string, 0, len(base))
	for _, p := range base {
		if !dropSet[p] {
			out = append(out, p)
		}
	}
	return out
}

// ---------------- 数据范围(OrgScope) ----------------

// OrgScope 是按组织过滤数据时用到的集合。一个用户对应一个 scope,各 controller
// 按自己页面的语义选用 UidSet / UserIdSet / OrgCode 之一。
type OrgScope struct {
	OrgCode   string   // 用户自身 org_code,projects 等按这个过滤
	UidSet    []string // 本 org 全成员的 uid + related_uids 并集,使用日志按 client_user_id 过滤时用
	UserIdSet []int    // 本 org 全成员的 users.id 集合,账单/消耗统计按 user_id 过滤时用
}

// CurrentUserOrgScope 给 controller 用的便捷封装:
//   - 系统 admin/root 返回 (nil, nil),controller 跳过 scope 过滤(看全局)
//   - 普通用户返回当前 org 的 scope,controller 据此加 WHERE
//
// 错误场景(user 查不到)也返回 nil scope + error,调用方决定是返回错误还是兜底空 scope。
func CurrentUserOrgScope(userID int, role int) (*OrgScope, error) {
	if role >= common.RoleAdminUser {
		return nil, nil
	}
	user, err := model.GetUserById(userID, false)
	if err != nil || user == nil {
		return nil, err
	}
	s := ComputeOrgScope(user)
	return &s, nil
}

// IsOrgMainUid 校验 client_user_id 是否恰好是某 org 内某成员的**主 uid**(不展开 related_uids)。
// 用于 mt-admin 创建 UID 预算条目时的校验:必须给某个"正式 mt 成员"开预算,
// 不能给"成员的关联 uid"开。
func IsOrgMainUid(orgCode string, clientUserId string) bool {
	if orgCode == "" || clientUserId == "" {
		return false
	}
	var count int64
	if err := model.DB.Model(&model.User{}).
		Where("org_code = ? AND uid = ?", orgCode, clientUserId).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

// ComputeOrgScope 算出当前用户的数据范围。
//
// 用于 mt-admin / wl-admin 等"看组织全员数据"的场景。系统 admin / root **不应**
// 调用这个函数(他们看全局,在 controller 里用 isAdmin() 分支跳过 scope 过滤)。
//
// 第一版每次请求查一次 SELECT users WHERE org_code=?,接受性能开销。
// mt/wl 成员 > 100 后再加 in-process cache(invalidate 时机:用户 org 变更、related_uids 变更)。
func ComputeOrgScope(user *model.User) OrgScope {
	scope := OrgScope{OrgCode: user.OrgCode}
	if user.OrgCode == "" {
		// 空 org 等同 other,没有组织聚合数据,scope 为空
		return scope
	}

	// 拉本 org 所有用户的 id / uid / related_uids
	var rows []struct {
		ID          int    `gorm:"column:id"`
		Uid         string `gorm:"column:uid"`
		RelatedUids string `gorm:"column:related_uids"`
	}
	if err := model.DB.Model(&model.User{}).
		Select("id, uid, related_uids").
		Where("org_code = ?", user.OrgCode).
		Find(&rows).Error; err != nil {
		logger.LogError(context.Background(), "ComputeOrgScope query failed: "+err.Error())
		return scope
	}

	uidSeen := make(map[string]bool)
	for _, r := range rows {
		scope.UserIdSet = append(scope.UserIdSet, r.ID)
		if r.Uid != "" && !uidSeen[r.Uid] {
			uidSeen[r.Uid] = true
			scope.UidSet = append(scope.UidSet, r.Uid)
		}
		// 展开 related_uids JSON 数组
		if r.RelatedUids != "" {
			var related []string
			if err := common.UnmarshalJsonStr(r.RelatedUids, &related); err == nil {
				for _, u := range related {
					if u != "" && !uidSeen[u] {
						uidSeen[u] = true
						scope.UidSet = append(scope.UidSet, u)
					}
				}
			}
		}
	}
	return scope
}
