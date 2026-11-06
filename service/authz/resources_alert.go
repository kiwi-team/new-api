package authz

var AlertReadAll = Permission{Resource: "alert", Action: "read_all"}
var AlertHandleAll = Permission{Resource: "alert", Action: "handle_all"}
var AlertEngineRead = Permission{Resource: "alert", Action: "read_engine_health"}

func init() {
	RegisterResource(ResourceDefinition{Resource: "alert", LabelKey: "Alert center", Actions: []ActionDefinition{
		{Action: "read_all", LabelKey: "View all alerts", DescriptionKey: "View alert rules and delivery history across accounts.", DefaultRoles: []string{BuiltInRoleAdmin}},
		{Action: "manage_others", LabelKey: "Manage other users' alerts", DescriptionKey: "Reserved for platform alert management."},
		{Action: "manage_platform", LabelKey: "Manage platform alerts", DescriptionKey: "Reserved for channel and system alert rules."},
		{Action: "handle_all", LabelKey: "Handle all alerts", DescriptionKey: "Acknowledge and close platform alert incidents."},
		{Action: "read_engine_health", LabelKey: "View alert engine health", DescriptionKey: "View alert processing coverage and delivery backlog.", DefaultRoles: []string{BuiltInRoleAdmin}},
	}})
}
