package model

import "github.com/QuantumNous/new-api/common"

// Deployment operators bound rule/target cardinality on every node. SQL owner
// locks still enforce the cap when multiple clients create objects concurrently.
func AlertRuleOwnerLimit() int {
	return min(max(common.GetEnvOrDefault("ALERTING_MAX_RULES_PER_OWNER", 200), 1), 10000)
}

func AlertTargetOwnerLimit() int {
	return min(max(common.GetEnvOrDefault("ALERTING_MAX_TARGETS_PER_OWNER", 100), 1), 1000)
}
