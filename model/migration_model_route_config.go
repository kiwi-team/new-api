package model

// MigrateModelRouteConfig 迁移模型路由配置表
func MigrateModelRouteConfig() error {
	return DB.AutoMigrate(&ModelRouteConfig{})
}
