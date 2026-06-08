package constant

// OrgInfo 描述一个组织：code 是唯一识别(用于 users.org_code)，name 是展示用，
// Enabled=false 表示该组织已停用(前端下拉里不出现，root 不应分配新用户进去)。
type OrgInfo struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// Orgs 组织清单。新增 / 启停组织 = 改这里 + 改 service.GetUserMenu 的 switch + 重新部署。
//
// 设计上故意不建 DB 表：权限策略本身已经 hardcode 在 service/org_view.go 里，
// 新增组织本就要改代码，DB 表只剩"name 展示 + enabled 开关"两个轻量需求，
// 完全够用常量表达。详见 org.md 第 3.1 节。
var Orgs = []OrgInfo{
	{Code: "mt", Name: "MT", Enabled: true},
	{Code: "wl", Name: "WL", Enabled: true},
	{Code: "other", Name: "其他", Enabled: true},
}

// 组织内角色常量。约定三档强制存在，未来扩展时新增字符串即可，
// service.GetUserMenu 里的 switch 会兜底到普通用户行为。
// mtuser 是 mt org 专属的"只看日志/统计"观察员角色:能看到 mt 全员 uid 数据范围,
// 但没有 UID 预算/项目预算菜单。
const (
	OrgRoleMember = "member"
	OrgRoleLeader = "leader"
	OrgRoleAdmin  = "admin"
	OrgRoleMtUser = "mtuser"
)

// IsValidOrgCode 校验 root 给用户分配 org_code 时输入合法。
// 空串("")等同 "other"(普通用户体验)，也算合法，方便老数据兜底。
func IsValidOrgCode(code string) bool {
	if code == "" {
		return true
	}
	for _, o := range Orgs {
		if o.Enabled && o.Code == code {
			return true
		}
	}
	return false
}

// IsValidOrgRole 校验 org_role 输入合法。
func IsValidOrgRole(role string) bool {
	switch role {
	case OrgRoleMember, OrgRoleLeader, OrgRoleAdmin, OrgRoleMtUser:
		return true
	}
	return false
}

// EnabledOrgs 返回当前可分配的组织清单，供前端下拉用。
func EnabledOrgs() []OrgInfo {
	out := make([]OrgInfo, 0, len(Orgs))
	for _, o := range Orgs {
		if o.Enabled {
			out = append(out, o)
		}
	}
	return out
}
