package model

import (
	"crypto/sha256"
	"fmt"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// AlertRuleSpec is the versioned public rule contract. Credentials belong to
// notification targets, never to rules, observations, or delivery jobs.
type AlertRuleSpec struct {
	QuotaUnit       int64                `json:"quota_unit"`
	Name            string               `json:"name"`
	Category        string               `json:"category"`
	Severity        string               `json:"severity"`
	Enabled         bool                 `json:"enabled"`
	ManagementMode  string               `json:"management_mode"`
	Scope           AlertScope           `json:"scope"`
	Condition       AlertCondition       `json:"condition"`
	Trigger         AlertTrigger         `json:"trigger"`
	Notify          AlertNotify          `json:"notify"`
	Actions         []AlertAction        `json:"actions"`
	ActionGuard     *AlertActionGuard    `json:"action_guard,omitempty"`
	Recovery        AlertRecovery        `json:"recovery"`
	Acknowledgement AlertAcknowledgement `json:"acknowledgement"`
}

type AlertScope struct {
	Type          string             `json:"type"`
	Selector      string             `json:"selector,omitempty"`
	EntityIDs     []int              `json:"entity_ids,omitempty"`
	ChannelIDs    []int              `json:"channel_ids,omitempty"`
	GroupMode     string             `json:"group_mode"`
	IncludeFuture bool               `json:"include_future"`
	ModelSelector AlertModelSelector `json:"model_selector"`
}

type AlertModelSelector struct {
	Mode          string   `json:"mode"`
	Models        []string `json:"models"`
	IncludeFuture bool     `json:"include_future_models"`
}

// AlertTrafficBaseline defines calls per current rolling window. Historical
// windows end where the current window begins, so they never overlap.
type AlertTrafficBaseline struct {
	Mode           string `json:"mode"`
	Count          int64  `json:"count,omitempty"`
	HistorySeconds int64  `json:"history_seconds,omitempty"`
	MinimumCount   int64  `json:"minimum_count,omitempty"`
}

type AlertCondition struct {
	Baseline               *AlertTrafficBaseline `json:"baseline,omitempty"`
	ChangeDirection        string                `json:"change_direction,omitempty"`
	LegacySignal           string                `json:"legacy_signal,omitempty"`
	BalancePool            string                `json:"balance_pool,omitempty"`
	SubscriptionID         int                   `json:"subscription_id,omitempty"`
	Metric                 string                `json:"metric"`
	Aggregation            string                `json:"aggregation,omitempty"`
	LatencyBasis           string                `json:"latency_basis,omitempty"`
	Operator               string                `json:"operator"`
	ThresholdMS            int64                 `json:"threshold_ms,omitempty"`
	ThresholdCount         int64                 `json:"threshold_count,omitempty"`
	ThresholdBPS           int64                 `json:"threshold_bps,omitempty"`
	ThresholdUSD           string                `json:"threshold_usd,omitempty"`
	RecoveryThresholdMS    int64                 `json:"recovery_threshold_ms,omitempty"`
	RecoveryThresholdCount int64                 `json:"recovery_threshold_count,omitempty"`
	RecoveryThresholdBPS   int64                 `json:"recovery_threshold_bps,omitempty"`
	RecoveryThresholdUSD   string                `json:"recovery_threshold_usd,omitempty"`
	SlowThresholdMS        int64                 `json:"slow_threshold_ms,omitempty"`
	MinSamples             int                   `json:"min_samples,omitempty"`
	HoldSeconds            int64                 `json:"hold_seconds"`
	RecoverySeconds        int64                 `json:"recovery_seconds"`
	DeadlineKind           string                `json:"deadline_kind,omitempty"`
	DeadlineMS             int64                 `json:"deadline_ms,omitempty"`
	Window                 AlertWindow           `json:"window"`
	Protocol               string                `json:"protocol,omitempty"`
	TaskType               string                `json:"task_type,omitempty"`
	Stream                 *bool                 `json:"stream,omitempty"`
}

type AlertWindow struct {
	Type            string `json:"type"`
	DurationSeconds int64  `json:"duration_seconds"`
	Timezone        string `json:"timezone"`
	SendAt          string `json:"send_at,omitempty"`
}

type AlertReportOptions struct {
	IncludeZero         *bool `json:"include_zero,omitempty"`
	AllowFirstPartial   bool  `json:"allow_first_partial"`
	BackfillPeriods     int   `json:"backfill_periods"`
	PartialAfterSeconds int64 `json:"partial_after_seconds"`
	SendRevision        bool  `json:"send_revision"`
}

type AlertTrigger struct {
	Report        AlertReportOptions `json:"report"`
	Mode          string             `json:"mode"`
	RepeatSeconds int64              `json:"repeat_seconds"`
	StepUSD       string             `json:"step_usd,omitempty"`
}

type AlertNotify struct {
	TargetIDs  []int64 `json:"target_ids"`
	OnRecovery bool    `json:"on_recovery"`
}

type AlertAction struct {
	Type   string `json:"type"`
	Target string `json:"target"`
}

type AlertActionGuard struct {
	MinRemainingChannels int    `json:"min_remaining_channels"`
	OnNoAlternative      string `json:"on_no_alternative"`
}

type AlertRecovery struct {
	Protocol        string   `json:"protocol"`
	Models          []string `json:"models,omitempty"`
	Mode            string   `json:"mode"`
	TTFTLimitMS     int64    `json:"ttft_limit_ms"`
	DurationLimitMS int64    `json:"duration_limit_ms"`
	Successes       int      `json:"successes"`
	ObserveSeconds  int64    `json:"observe_seconds"`
}

type AlertAcknowledgement struct {
	Required             bool    `json:"required"`
	RemindAfterSeconds   int64   `json:"remind_after_seconds"`
	EscalateAfterSeconds int64   `json:"escalate_after_seconds"`
	EscalationTargetIDs  []int64 `json:"escalation_target_ids"`
}

// Alert timestamps use epoch milliseconds. UpdatedAt is set explicitly so a
// consumer increment cannot masquerade as a fresh producer heartbeat.
type AlertRule struct {
	LastEvaluationError string         `json:"last_evaluation_error" gorm:"size:64"`
	LastEvaluatedAt     int64          `json:"last_evaluated_at"`
	ID                  int64          `json:"id" gorm:"primaryKey"`
	OwnerUserID         int            `json:"owner_user_id" gorm:"index"`
	CreatedBy           int            `json:"created_by"`
	UpdatedBy           int            `json:"updated_by"`
	Name                string         `json:"name" gorm:"size:160"`
	Enabled             bool           `json:"enabled" gorm:"index:idx_alert_rule_due,priority:1"`
	Revision            int64          `json:"revision"`
	Generation          int64          `json:"condition_generation"`
	EffectiveAt         int64          `json:"effective_at"`
	NextEvaluationAt    int64          `json:"next_evaluation_at" gorm:"index:idx_alert_rule_due,priority:2"`
	EvaluationCursor    string         `json:"-" gorm:"size:64"`
	Config              string         `json:"-" gorm:"type:text"`
	CreatedAt           int64          `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt           int64          `json:"updated_at" gorm:"autoUpdateTime:false"`
	DeletedAt           gorm.DeletedAt `json:"-" gorm:"index"`
}

type AlertRuleRevision struct {
	ID        int64  `json:"id" gorm:"primaryKey"`
	RuleID    int64  `json:"rule_id" gorm:"uniqueIndex:idx_alert_revision,priority:1"`
	Revision  int64  `json:"revision" gorm:"uniqueIndex:idx_alert_revision,priority:2"`
	Config    string `json:"-" gorm:"type:text"`
	CreatedBy int    `json:"created_by"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime:milli"`
}

type AlertTarget struct {
	ID             int64          `json:"id" gorm:"primaryKey"`
	OwnerUserID    int            `json:"owner_user_id" gorm:"index"`
	Name           string         `json:"name" gorm:"size:160"`
	Type           string         `json:"type" gorm:"size:24"`
	AddressHint    string         `json:"address_hint" gorm:"size:255"`
	RateKey        string         `json:"-" gorm:"size:64"`
	Ciphertext     string         `json:"-" gorm:"type:text"`
	KeyID          string         `json:"key_id" gorm:"size:32"`
	Revision       int64          `json:"revision"`
	Enabled        bool           `json:"enabled"`
	Language       string         `json:"language" gorm:"size:16"`
	LastTestAt     int64          `json:"last_test_at"`
	LastTestStatus string         `json:"last_test_status" gorm:"size:24"`
	CreatedAt      int64          `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt      int64          `json:"updated_at" gorm:"autoUpdateTime:false"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
}

// AlertObservation stores accepted, immutable facts in the primary database.
// Namespace/SourceID distinguish lifecycle events from one terminal outcome.
type AlertObservation struct {
	ID                 string  `json:"id" gorm:"primaryKey;size:64"`
	PayloadHash        string  `json:"-" gorm:"size:64"`
	ReceiptNonce       string  `json:"-" gorm:"size:64"`
	TerminalKey        *string `json:"-" gorm:"size:64;uniqueIndex"`
	SourcePositionKey  *string `json:"-" gorm:"size:64;uniqueIndex"`
	Namespace          string  `json:"namespace" gorm:"size:24;index:idx_alert_fact_namespace,priority:1"`
	SourceID           string  `json:"source_id" gorm:"size:64;index"`
	Kind               string  `json:"kind" gorm:"size:24"`
	Layer              string  `json:"layer" gorm:"size:24"`
	OccurredAt         int64   `json:"occurred_at" gorm:"index:idx_alert_fact_time;index:idx_alert_fact_user,priority:2;index:idx_alert_fact_token,priority:2;index:idx_alert_fact_channel,priority:2;index:idx_alert_fact_model,priority:3;index:idx_alert_fact_namespace,priority:2"`
	AcceptedAt         int64   `json:"accepted_at"`
	UserID             int     `json:"user_id" gorm:"index:idx_alert_fact_user,priority:1"`
	TokenID            int     `json:"token_id" gorm:"index:idx_alert_fact_token,priority:1"`
	ChannelID          int     `json:"channel_id" gorm:"index:idx_alert_fact_channel,priority:1;index:idx_alert_fact_model,priority:1"`
	ChannelName        string  `json:"channel_name" gorm:"type:text"`
	ModelKey           string  `json:"model_key" gorm:"type:text"`
	ModelHash          string  `json:"-" gorm:"size:64;index:idx_alert_fact_model,priority:2"`
	Protocol           string  `json:"protocol" gorm:"size:32"`
	Stream             bool    `json:"stream"`
	Outcome            string  `json:"outcome" gorm:"size:16"`
	SampleClass        string  `json:"sample_class" gorm:"size:24"`
	DurationMS         *int64  `json:"duration_ms"`
	TTFTMS             *int64  `json:"ttft_ms" gorm:"column:ttft_ms"`
	Quota              int64   `json:"quota,string"`
	RequestCount       int64   `json:"request_count,string"`
	InputTokens        int64   `json:"input_tokens,string"`
	OutputTokens       int64   `json:"output_tokens,string"`
	CachedTokens       int64   `json:"cached_tokens,string"`
	CacheWrite5mTokens int64   `json:"cache_write_5m_tokens,string"`
	CacheWrite1hTokens int64   `json:"cache_write_1h_tokens,string"`
	RequestID          string  `json:"request_id" gorm:"size:64"`
	NodeID             string  `json:"node_id" gorm:"size:160"`
	BootID             string  `json:"boot_id" gorm:"size:64;index:idx_alert_source_position,priority:1"`
	Sequence           int64   `json:"sequence"`
	StartedAt          int64   `json:"started_at"`
	FirstOutputAt      int64   `json:"first_output_at"`
	LastOutputAt       int64   `json:"last_output_at"`
	OutputCovered      bool    `json:"output_covered"`
	AttributionCovered bool    `json:"attribution_covered"`
	ConfigVersion      string  `json:"config_version" gorm:"size:64"`
	SourceSequence     int64   `json:"source_sequence" gorm:"index:idx_alert_source_position,priority:2"`
	ClientUserID       string  `json:"client_user_id" gorm:"type:text"`
	ClientScenario     string  `json:"client_scenario" gorm:"type:text"`
	ProjectName        string  `json:"project_name" gorm:"type:text"`
	ProjectID          int     `json:"project_id"`
	PlanID             int     `json:"plan_id"`
}

type AlertLifecycle struct {
	ID            string `json:"id" gorm:"primaryKey;size:64"`
	Layer         string `json:"layer" gorm:"size:24;index:idx_alert_life_active,priority:3"`
	SampleClass   string `json:"sample_class" gorm:"size:24;index:idx_alert_life_active,priority:2"`
	RequestID     string `json:"request_id" gorm:"size:64"`
	ChannelID     int    `json:"channel_id" gorm:"index:idx_alert_life_channel,priority:1;index:idx_alert_life_model,priority:1"`
	ChannelName   string `json:"channel_name" gorm:"type:text"`
	ModelKey      string `json:"model_key" gorm:"type:text"`
	ModelHash     string `json:"-" gorm:"size:64;index:idx_alert_life_model,priority:2"`
	Protocol      string `json:"protocol" gorm:"size:32"`
	Stream        bool   `json:"stream"`
	UserID        int    `json:"user_id"`
	TokenID       int    `json:"token_id"`
	NodeID        string `json:"node_id" gorm:"size:160"`
	BootID        string `json:"boot_id" gorm:"size:64;index"`
	StartedAt     int64  `json:"started_at" gorm:"index:idx_alert_life_active,priority:4;index:idx_alert_life_channel,priority:3;index:idx_alert_life_model,priority:4"`
	FirstOutputAt int64  `json:"first_output_at"`
	LastOutputAt  int64  `json:"last_output_at"`
	OutputCovered bool   `json:"output_covered"`
	ConfigVersion string `json:"config_version" gorm:"size:64"`
	FinishedAt    int64  `json:"finished_at" gorm:"index:idx_alert_life_active,priority:1;index:idx_alert_life_channel,priority:2;index:idx_alert_life_model,priority:3"`
	Outcome       string `json:"outcome" gorm:"size:16"`
	Sequence      int64  `json:"sequence"`
	UpdatedAt     int64  `json:"updated_at" gorm:"autoUpdateTime:false"`
}

type AlertRuleState struct {
	TargetName     string `json:"target_name" gorm:"type:text"`
	ObservedAt     int64  `json:"observed_at"`
	ConfigVersion  string `json:"config_version" gorm:"size:64"`
	ID             string `json:"id" gorm:"primaryKey;size:64"`
	RuleID         int64  `json:"rule_id" gorm:"index"`
	Generation     int64  `json:"generation"`
	TargetType     string `json:"target_type" gorm:"size:24"`
	TargetID       int    `json:"target_id"`
	ModelKey       string `json:"model_key" gorm:"type:text"`
	Status         string `json:"status" gorm:"size:24"`
	DataStatus     string `json:"data_status" gorm:"size:24"`
	PendingAt      int64  `json:"pending_at"`
	RecoveryAt     int64  `json:"recovery_at"`
	LastValue      string `json:"last_value" gorm:"size:64"`
	LastCount      int64  `json:"last_count,string"`
	Baseline       int64  `json:"baseline,string"`
	HighestTier    int64  `json:"highest_tier,string"`
	PeriodKey      string `json:"period_key" gorm:"size:32"`
	IncidentID     int64  `json:"incident_id"`
	LastNotifiedAt int64  `json:"last_notified_at"`
	UpdatedAt      int64  `json:"updated_at" gorm:"autoUpdateTime:false"`
}

type AlertIncident struct {
	ID             int64  `json:"id" gorm:"primaryKey"`
	RuleID         int64  `json:"rule_id" gorm:"index"`
	OwnerUserID    int    `json:"owner_user_id" gorm:"index"`
	TargetKey      string `json:"target_key" gorm:"size:64;index"`
	TargetName     string `json:"target_name" gorm:"type:text"`
	Severity       string `json:"severity" gorm:"size:4;index"`
	Status         string `json:"status" gorm:"size:24;index"`
	HandlingStatus string `json:"handling_status" gorm:"size:24"`
	AcknowledgedBy int    `json:"acknowledged_by"`
	AcknowledgedAt int64  `json:"acknowledged_at"`
	ClosedBy       int    `json:"closed_by"`
	ClosedAt       int64  `json:"closed_at"`
	CloseReason    string `json:"close_reason" gorm:"type:text"`
	OpenedAt       int64  `json:"opened_at"`
	ResolvedAt     int64  `json:"resolved_at"`
	UpdatedAt      int64  `json:"updated_at" gorm:"autoUpdateTime:false"`
}

type AlertEvent struct {
	ReceiptNonce         string `json:"-" gorm:"size:64"`
	MilestoneFirst       int64  `json:"milestone_first,string"`
	MilestoneLast        int64  `json:"milestone_last,string"`
	ID                   int64  `json:"id" gorm:"primaryKey"`
	DedupKey             string `json:"-" gorm:"size:64;uniqueIndex"`
	RuleID               int64  `json:"rule_id" gorm:"index"`
	IncidentID           int64  `json:"incident_id" gorm:"index"`
	OwnerUserID          int    `json:"owner_user_id" gorm:"index"`
	Kind                 string `json:"kind" gorm:"size:24"`
	Severity             string `json:"severity" gorm:"size:4"`
	TargetName           string `json:"target_name" gorm:"type:text"`
	Summary              string `json:"summary" gorm:"type:text"`
	Evidence             string `json:"evidence" gorm:"type:text"`
	RuleConfig           string `json:"-" gorm:"type:text"`
	ConditionSatisfiedAt int64  `json:"condition_satisfied_at"`
	ThresholdCrossedAt   int64  `json:"threshold_crossed_at"`
	SourceAcceptedAt     int64  `json:"source_accepted_at"`
	TimingBasis          string `json:"timing_basis" gorm:"size:40"`
	CreatedAt            int64  `json:"created_at" gorm:"autoCreateTime:milli;index"`
}

type AlertJob struct {
	TargetName       string `json:"target_name" gorm:"type:text"`
	TargetType       string `json:"target_type" gorm:"size:24"`
	TargetHint       string `json:"target_hint" gorm:"size:255"`
	Platform         bool   `json:"platform_enforced" gorm:"index:idx_alert_job_platform,priority:1"`
	ID               int64  `json:"id" gorm:"primaryKey"`
	DedupKey         string `json:"-" gorm:"size:64;uniqueIndex"`
	EventID          int64  `json:"event_id" gorm:"index"`
	RuleID           int64  `json:"rule_id" gorm:"index"`
	RuleRevision     int64  `json:"rule_revision"`
	Kind             string `json:"kind" gorm:"size:16;index:idx_alert_job_due,priority:1;index:idx_alert_job_platform,priority:2"`
	TargetID         int64  `json:"target_id"`
	TargetRevision   int64  `json:"target_revision"`
	Action           string `json:"action" gorm:"size:32"`
	Payload          string `json:"-" gorm:"type:text"`
	Priority         int    `json:"priority"`
	Status           string `json:"status" gorm:"size:24;index:idx_alert_job_due,priority:2;index:idx_alert_job_platform,priority:3"`
	AvailableAt      int64  `json:"available_at" gorm:"index:idx_alert_job_due,priority:3;index:idx_alert_job_platform,priority:4"`
	ExpiresAt        int64  `json:"expires_at"`
	LockedBy         string `json:"-" gorm:"size:64"`
	LockedUntil      int64  `json:"locked_until"`
	Attempts         int    `json:"attempts"`
	LastError        string `json:"last_error" gorm:"size:255"`
	AcceptedAt       int64  `json:"accepted_at"`
	RenderedContent  string `json:"rendered_content,omitempty" gorm:"type:text"`
	RenderedLanguage string `json:"rendered_language,omitempty" gorm:"size:16"`
	CreatedAt        int64  `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt        int64  `json:"updated_at" gorm:"autoUpdateTime:false"`
}

type AlertJobAttempt struct {
	ID         int64  `json:"id" gorm:"primaryKey"`
	JobID      int64  `json:"job_id" gorm:"index"`
	Status     string `json:"status" gorm:"size:24"`
	HTTPStatus int    `json:"http_status"`
	ErrorCode  string `json:"error_code" gorm:"size:64"`
	DurationMS int64  `json:"duration_ms"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime:milli"`
}

type AlertDisableHold struct {
	ID                     int64  `json:"id" gorm:"primaryKey"`
	DedupKey               string `json:"-" gorm:"size:64;uniqueIndex"`
	TargetKey              string `json:"target_key" gorm:"size:64;index"`
	TargetType             string `json:"target_type" gorm:"size:24"`
	TargetName             string `json:"target_name" gorm:"type:text"`
	ChannelID              int    `json:"channel_id" gorm:"index"`
	TokenID                int    `json:"token_id" gorm:"index"`
	ModelKey               string `json:"model_key" gorm:"type:text"`
	RuleID                 int64  `json:"rule_id" gorm:"index"`
	IncidentID             int64  `json:"incident_id"`
	Active                 bool   `json:"active" gorm:"index"`
	ConfigVersion          string `json:"config_version" gorm:"size:64"`
	CreatedAt              int64  `json:"created_at" gorm:"autoCreateTime:milli"`
	ReleasedAt             int64  `json:"released_at"`
	ReleasedBy             int    `json:"released_by"`
	OwnerUserID            int    `json:"owner_user_id" gorm:"index"`
	Platform               bool   `json:"platform_enforced"`
	TokenVersion           int64  `json:"token_version"`
	Recovery               string `json:"recovery" gorm:"type:text"`
	ProbeCount             int    `json:"probe_count"`
	ProbeStartedAt         int64  `json:"probe_started_at"`
	NextProbeAt            int64  `json:"next_probe_at" gorm:"index"`
	LastProbeAt            int64  `json:"last_probe_at"`
	LastProbeStatus        string `json:"last_probe_status" gorm:"size:32"`
	ProbeCredentialVersion string `json:"-" gorm:"size:64"`
}

const AlertSourceProtocolVersion = 2

type AlertWorkerCheckpoint struct {
	UsageHealthy      bool   `json:"usage_healthy"`
	CoverageStartedAt int64  `json:"coverage_started_at"`
	ProtocolVersion   int    `json:"protocol_version"`
	ID                string `json:"id" gorm:"primaryKey;size:64"`
	NodeID            string `json:"node_id" gorm:"size:160;index:idx_alert_source_node,priority:1"`
	BootID            string `json:"boot_id" gorm:"size:64"`
	Kind              string `json:"kind" gorm:"size:24;index:idx_alert_source_node,priority:2"`
	UpdatedAt         int64  `json:"updated_at" gorm:"autoUpdateTime:false;index;index:idx_alert_source_node,priority:3"`
	Healthy           bool   `json:"healthy"`
	LastError         string `json:"last_error" gorm:"size:160"`
	OfferedSequence   int64  `json:"offered_sequence"`
	AcceptedCount     int64  `json:"accepted_count"`
	AcceptedThrough   int64  `json:"accepted_through"`
	CapturedAt        int64  `json:"captured_at"`
	LastCompleteAt    int64  `json:"last_complete_at"`
}

// Policy is separate from declared capabilities: rebuilding abilities or
// editing a channel must never erase manual or alert isolation.
type AlertChannelPolicy struct {
	ChannelID      int    `json:"channel_id" gorm:"primaryKey;autoIncrement:false"`
	ProtectionMode string `json:"protection_mode" gorm:"size:16"`
	Revision       int64  `json:"revision"`
	UpdatedBy      int    `json:"updated_by"`
	UpdatedAt      int64  `json:"updated_at" gorm:"autoUpdateTime:false"`
}

type AlertChannelModelControl struct {
	TargetKey    string `json:"target_key" gorm:"primaryKey;size:64"`
	ChannelID    int    `json:"channel_id" gorm:"index"`
	ModelKey     string `json:"model_key" gorm:"type:text"`
	ManualPaused bool   `json:"manual_paused"`
	Revision     int64  `json:"revision"`
	UpdatedBy    int    `json:"updated_by"`
	UpdatedAt    int64  `json:"updated_at" gorm:"autoUpdateTime:false"`
}

// AlertDeployment isolates deployments sharing a Redis service or local spool root.
// Every node using the same primary database reads the same namespace.
type AlertReportPeriod struct {
	ID            string `gorm:"primaryKey;size:64"`
	RuleID        int64  `gorm:"index"`
	Generation    int64
	TargetKey     string `gorm:"size:64"`
	WindowStart   int64
	WindowEnd     int64
	InitialStatus string `gorm:"size:16"`
	RevisedAt     int64
	CreatedAt     int64 `gorm:"autoCreateTime:milli"`
}

type AlertDeployment struct {
	ID        int    `gorm:"primaryKey;autoIncrement:false"`
	Namespace string `gorm:"size:64"`
}

type AlertOutbox struct {
	ID          string `json:"id" gorm:"primaryKey;size:64"`
	Kind        string `json:"kind" gorm:"size:24"`
	Reference   string `json:"reference" gorm:"size:64"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime:milli;index"`
	PublishedAt int64  `json:"published_at" gorm:"index"`
}

type AlertDeadLetter struct {
	ID          string `gorm:"primaryKey;size:64"`
	Source      string `gorm:"size:64"`
	PayloadHash string `gorm:"size:64"`
	ErrorCode   string `gorm:"size:48"`
	CreatedAt   int64  `gorm:"autoCreateTime:milli;index"`
}

type AlertAudit struct {
	ID          int64  `json:"id" gorm:"primaryKey"`
	ActorID     int    `json:"actor_id"`
	OwnerUserID int    `json:"owner_user_id" gorm:"index"`
	Operation   string `json:"operation" gorm:"size:48"`
	ObjectID    int64  `json:"object_id"`
	Revision    int64  `json:"revision"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime:milli;index"`
}

type AlertNotificationLimit struct {
	ID        string `gorm:"primaryKey;size:64"`
	Count     int
	ExpiresAt int64 `gorm:"index"`
}

// AlertIdentity uses a bounded, case-sensitive identity on every SQL dialect.
func AlertIdentity(parts ...any) string {
	b, _ := common.Marshal(parts) // callers pass only primitive, bounded identities
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func AlertModels() []any {
	return []any{&AlertRule{}, &AlertRuleRevision{}, &AlertTarget{}, &AlertObservation{}, &AlertLifecycle{}, &AlertRuleState{}, &AlertIncident{}, &AlertEvent{}, &AlertJob{}, &AlertJobAttempt{}, &AlertDisableHold{}, &AlertWorkerCheckpoint{}, &AlertChannelPolicy{}, &AlertChannelModelControl{}, &AlertOutbox{}, &AlertAudit{}, &AlertNotificationLimit{}, &AlertDeadLetter{}, &AlertDeployment{}, &AlertReportPeriod{}, &AlertMigration{}, &AlertMetricBucket{}}
}

func MigrateAlertModels(db *gorm.DB) error { return db.AutoMigrate(AlertModels()...) }

// AlertMigration keeps the old and new executors mutually exclusive. Baseline
// offsets are internal quota units, never wallet adjustments or refunds.
type AlertMigration struct {
	ID               string `json:"id" gorm:"primaryKey;size:64"`
	Kind             string `json:"kind" gorm:"size:32"`
	ObjectID         string `json:"object_id" gorm:"size:160"`
	OwnerUserID      int    `json:"owner_user_id" gorm:"index"`
	RuleID           int64  `json:"rule_id" gorm:"uniqueIndex"`
	Executor         string `json:"executor" gorm:"size:16"`
	Status           string `json:"status" gorm:"size:24"`
	Revision         int64  `json:"revision"`
	MetricOffset     int64  `json:"metric_offset,string"`
	MetricStartAt    int64  `json:"metric_start_at"`
	Checkpoint       string `json:"checkpoint" gorm:"type:text"`
	LegacyConfigHash string `json:"legacy_config_hash" gorm:"size:64"`
	ImportedAt       int64  `json:"imported_at"`
	CutoverAt        int64  `json:"cutover_at"`
	UpdatedAt        int64  `json:"updated_at" gorm:"autoUpdateTime:false"`
	UpdatedBy        int    `json:"updated_by"`
}
