package model

import (
	"time"

	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	HappyOysterTicketIssued   = "issued"
	HappyOysterTicketConsumed = "consumed"
	HappyOysterTicketUnknown  = "unknown"

	HappyOysterSettlementPending = "pending"
	HappyOysterSettlementRunning = "settling"
	HappyOysterSettlementDone    = "done"
)

// HappyOysterWorld is the gateway-side routing record for an upstream world.
// Upstream credentials and tickets are deliberately never persisted here.
type HappyOysterWorld struct {
	Model                string `json:"model" gorm:"type:varchar(64);default:happyoyster-1.0-adventure;uniqueIndex:idx_happy_oyster_world_model_user"`
	Resolution           string `json:"resolution" gorm:"type:varchar(16)"`
	HasReferenceImage    bool   `json:"-"`
	Id                   int64  `json:"id"`
	UserId               int    `json:"user_id" gorm:"index;uniqueIndex:idx_happy_oyster_world_model_user"`
	TokenId              int    `json:"token_id" gorm:"index"`
	EncryptedWorldId     string `json:"encrypted_world_id" gorm:"type:varchar(255);uniqueIndex:idx_happy_oyster_world_model_user"`
	ChannelId            int    `json:"channel_id" gorm:"index"`
	ChannelMultiKeyIndex int    `json:"channel_multi_key_index"`
	ChannelAccountHash   string `json:"-" gorm:"type:varchar(64)"`
	BaseURL              string `json:"-" gorm:"type:text"`
	Status               string `json:"status" gorm:"type:varchar(32);index"`
	Name                 string `json:"name" gorm:"type:varchar(255)"`
	Perspective          string `json:"perspective" gorm:"type:varchar(32)"`
	CreationModel        string `json:"creation_model" gorm:"type:varchar(32)"`
	UploadMode           string `json:"upload_mode" gorm:"type:varchar(32)"`
	FirstFrame           string `json:"first_frame" gorm:"type:text"`
	Deleted              bool   `json:"-"`
	CreatedAt            int64  `json:"created_at"`
	UpdatedAt            int64  `json:"updated_at"`
}

type HappyOysterTicket struct {
	Id         int64  `json:"id"`
	TicketHash string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	WorldId    int64  `json:"world_id" gorm:"index"`
	UserId     int    `json:"user_id" gorm:"index"`
	TokenId    int    `json:"token_id"`
	State      string `json:"state" gorm:"type:varchar(16);index"`
	ExpiresAt  int64  `json:"expires_at" gorm:"index"`
	TravelId   int64  `json:"travel_id"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}

type HappyOysterTravel struct {
	Model                string `json:"model" gorm:"type:varchar(64);default:happyoyster-1.0-adventure;uniqueIndex:idx_happy_oyster_travel_model_user"`
	Resolution           string `json:"resolution" gorm:"type:varchar(16)"`
	CreationModel        string `json:"creation_model" gorm:"type:varchar(32)"`
	DurationKnown        bool   `json:"-"`
	ErrorCode            string `json:"error_code" gorm:"type:varchar(128)"`
	ErrorMessage         string `json:"error_message" gorm:"type:text"`
	LastPolledAt         int64  `json:"-" gorm:"index;default:0"`
	Id                   int64  `json:"id"`
	UserId               int    `json:"user_id" gorm:"index;uniqueIndex:idx_happy_oyster_travel_model_user"`
	TokenId              int    `json:"token_id" gorm:"index"`
	WorldRecordId        int64  `json:"world_record_id" gorm:"index"`
	EncryptedWorldId     string `json:"encrypted_world_id" gorm:"type:varchar(255);index"`
	EncryptedTravelId    string `json:"encrypted_travel_id" gorm:"type:varchar(255);uniqueIndex:idx_happy_oyster_travel_model_user"`
	ChannelId            int    `json:"channel_id" gorm:"index"`
	ChannelMultiKeyIndex int    `json:"channel_multi_key_index"`
	ChannelAccountHash   string `json:"-" gorm:"type:varchar(64)"`
	BaseURL              string `json:"-" gorm:"type:text"`
	Status               string `json:"status" gorm:"type:varchar(32);index"`
	MaxExperienceSeconds int    `json:"max_experience_time_sec"`
	DurationSeconds      int    `json:"duration_sec"`
	PreConsumedQuota     int    `json:"pre_consumed_quota"`
	ActualQuota          int    `json:"actual_quota"`
	BillingSource        string `json:"billing_source" gorm:"type:varchar(24)"`
	SubscriptionId       int    `json:"subscription_id"`
	BillingSnapshot      string `json:"-" gorm:"type:text"`
	UsingGroup           string `json:"-" gorm:"type:varchar(64)"`
	ClientUserId         string `json:"-" gorm:"type:varchar(255)"`
	ClientScenairo       string `json:"-" gorm:"type:varchar(255)"`
	ProjectId            int    `json:"-"`
	ProjectName          string `json:"-" gorm:"type:varchar(255)"`
	PlanId               int    `json:"-"`
	SettlementState      string `json:"settlement_state" gorm:"type:varchar(16);index"`
	EndedAt              string `json:"ended_at" gorm:"type:varchar(64)"`
	CreatedAt            int64  `json:"created_at"`
	UpdatedAt            int64  `json:"updated_at"`
}

type HappyOysterClientToken struct {
	Model                string `json:"model" gorm:"type:varchar(64);default:happyoyster-1.0-adventure"`
	Id                   int64  `json:"id"`
	TokenHash            string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	UserId               int    `json:"user_id" gorm:"index"`
	TokenId              int    `json:"token_id" gorm:"index"`
	WorldId              int64  `json:"world_id" gorm:"index"`
	ChannelId            int    `json:"channel_id" gorm:"index"`
	ChannelMultiKeyIndex int    `json:"channel_multi_key_index"`
	ChannelAccountHash   string `json:"-" gorm:"type:varchar(64)"`
	BaseURL              string `json:"-" gorm:"type:text"`
	Scopes               string `json:"scopes" gorm:"type:varchar(255)"`
	ExpiresAt            int64  `json:"expires_at" gorm:"index"`
	RevokedAt            int64  `json:"revoked_at"`
	CreatedAt            int64  `json:"created_at"`
}

func (w *HappyOysterWorld) BeforeCreate(*gorm.DB) error {
	now := time.Now().Unix()
	w.CreatedAt, w.UpdatedAt = now, now
	return nil
}

func (w *HappyOysterWorld) BeforeUpdate(*gorm.DB) error {
	w.UpdatedAt = time.Now().Unix()
	return nil
}
func (t *HappyOysterTicket) BeforeCreate(*gorm.DB) error {
	now := time.Now().Unix()
	t.CreatedAt, t.UpdatedAt = now, now
	return nil
}
func (t *HappyOysterTravel) BeforeCreate(*gorm.DB) error {
	now := time.Now().Unix()
	t.CreatedAt, t.UpdatedAt = now, now
	return nil
}
func (t *HappyOysterClientToken) BeforeCreate(*gorm.DB) error {
	t.CreatedAt = time.Now().Unix()
	return nil
}

func GetHappyOysterWorld(userId int, encryptedId string, models ...string) (*HappyOysterWorld, error) {
	var world HappyOysterWorld
	err := happyOysterModelQuery(DB, models).Where("user_id = ? AND encrypted_world_id = ? AND deleted = ?", userId, encryptedId, false).First(&world).Error
	return &world, err
}

func GetDeletedHappyOysterWorld(userId int, encryptedId string, models ...string) (*HappyOysterWorld, error) {
	var world HappyOysterWorld
	err := happyOysterModelQuery(DB, models).Where("user_id = ? AND encrypted_world_id = ? AND deleted = ?", userId, encryptedId, true).First(&world).Error
	return &world, err
}

func GetHappyOysterTravel(userId int, encryptedId string, models ...string) (*HappyOysterTravel, error) {
	var travel HappyOysterTravel
	err := happyOysterModelQuery(DB, models).Where("user_id = ? AND encrypted_travel_id = ?", userId, encryptedId).First(&travel).Error
	return &travel, err
}

func ClaimHappyOysterTicket(userId int, ticketHash string) (*HappyOysterTicket, error) {
	var ticket HappyOysterTicket
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("user_id = ? AND ticket_hash = ?", userId, ticketHash).First(&ticket).Error; err != nil {
			return err
		}
		if ticket.State != HappyOysterTicketIssued || ticket.ExpiresAt <= time.Now().Unix() {
			return gorm.ErrRecordNotFound
		}
		ticket.State = HappyOysterTicketUnknown
		ticket.UpdatedAt = time.Now().Unix()
		return tx.Model(&ticket).Select("state", "updated_at").Updates(&ticket).Error
	})
	return &ticket, err
}

func GetHappyOysterTicket(userId int, ticketHash string) (*HappyOysterTicket, error) {
	var ticket HappyOysterTicket
	err := DB.Where("user_id = ? AND ticket_hash = ? AND state = ? AND expires_at > ?", userId, ticketHash, HappyOysterTicketIssued, time.Now().Unix()).First(&ticket).Error
	return &ticket, err
}

func GetHappyOysterClientToken(tokenHash string) (*HappyOysterClientToken, error) {
	var token HappyOysterClientToken
	err := DB.Where("token_hash = ? AND revoked_at = 0 AND expires_at > ?", tokenHash, time.Now().Unix()).First(&token).Error
	return &token, err
}

func ClaimHappyOysterSettlement(travelId int64) (bool, error) {
	result := DB.Model(&HappyOysterTravel{}).
		Where("id = ? AND settlement_state = ?", travelId, HappyOysterSettlementPending).
		Updates(map[string]any{"settlement_state": HappyOysterSettlementRunning, "updated_at": time.Now().Unix()})
	return result.RowsAffected == 1, result.Error
}

func FinishHappyOysterSettlement(travelId int64, actualQuota, duration int, status string) error {
	return DB.Model(&HappyOysterTravel{}).
		Where("id = ? AND settlement_state = ?", travelId, HappyOysterSettlementRunning).
		Updates(map[string]any{"actual_quota": actualQuota, "duration_seconds": duration, "duration_known": true, "status": status, "settlement_state": HappyOysterSettlementDone, "updated_at": time.Now().Unix()}).Error
}

func RetryHappyOysterSettlement(travelId int64) {
	DB.Model(&HappyOysterTravel{}).
		Where("id = ? AND settlement_state = ?", travelId, HappyOysterSettlementRunning).
		Updates(map[string]any{"settlement_state": HappyOysterSettlementPending, "updated_at": time.Now().Unix()})
}

func ListHappyOysterWorlds(userId, page, pageSize int, status string, models ...string) ([]HappyOysterWorld, int64, error) {
	var rows []HappyOysterWorld
	var total int64
	query := happyOysterModelQuery(DB.Model(&HappyOysterWorld{}), models).Where("user_id = ? AND deleted = ?", userId, false)
	if status == "generating" || status == "ready" || status == "failed" {
		query = query.Where("status = ?", status)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}, Desc: true}).Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error
	return rows, total, err
}

func ListHappyOysterTravels(userId, page, pageSize int, status, encryptedWorldId string, models ...string) ([]HappyOysterTravel, int64, error) {
	var rows []HappyOysterTravel
	var total int64
	query := happyOysterModelQuery(DB.Model(&HappyOysterTravel{}), models).Where("user_id = ?", userId)
	if status == "init" || status == "pending" || status == "running" || status == "failed" || status == "completed" {
		query = query.Where("status = ?", status)
	}
	if encryptedWorldId != "" {
		query = query.Where("encrypted_world_id = ?", encryptedWorldId)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}, Desc: true}).Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error
	return rows, total, err
}

func HasUnsettledHappyOysterTravels() bool {
	var count int64
	DB.Model(&HappyOysterTravel{}).Where("settlement_state = ?", HappyOysterSettlementPending).Limit(1).Count(&count)
	return count > 0
}

func GetUnsettledHappyOysterTravels(limit int) []HappyOysterTravel {
	var rows []HappyOysterTravel
	DB.Where("settlement_state = ?", HappyOysterSettlementPending).Order("last_polled_at asc, id asc").Limit(limit).Find(&rows)
	return rows
}

func happyOysterModelQuery(db *gorm.DB, models []string) *gorm.DB {
	name := constant.HappyOysterAdventure
	if len(models) > 0 {
		name = constant.HappyOysterModel(models[0])
	}
	return db.Where("model = ?", name)
}

// HappyOysterInstruction prevents re-execution and duplicate charges when a
// client retries a billable instruction. Unknown outcomes deliberately stay pending.
type HappyOysterInstruction struct {
	Id               int64
	TravelId         int64  `gorm:"uniqueIndex:idx_happy_oyster_instruction"`
	ClientRequestId  string `gorm:"type:varchar(32)"`
	RequestKey       string `gorm:"type:varchar(64);uniqueIndex:idx_happy_oyster_instruction"`
	RequestHash      string `gorm:"type:varchar(64)"`
	State            string `gorm:"type:varchar(16)"`
	HTTPStatus       int
	Response         string `gorm:"type:text"`
	TokenId          int
	PreConsumedQuota int
	BillingSource    string `gorm:"type:varchar(24)"`
	SubscriptionId   int
	BillingSnapshot  string `gorm:"type:text"`
	CreatedAt        int64  `gorm:"autoCreateTime"`
}

// MigrateHappyOysterModels runs after AutoMigrate. The old unique indexes are
// removed only after their model-aware replacements have been created.
func MigrateHappyOysterModels(db *gorm.DB) error {
	for _, table := range []any{&HappyOysterWorld{}, &HappyOysterTravel{}, &HappyOysterClientToken{}} {
		if err := db.Model(table).Where("model IS NULL OR model = ?", "").Update("model", constant.HappyOysterAdventure).Error; err != nil {
			return err
		}
	}
	for _, table := range []any{&HappyOysterWorld{}, &HappyOysterTravel{}} {
		if err := db.Model(table).Where("model = ? AND (resolution IS NULL OR resolution = ?)", constant.HappyOysterAdventure, "").Update("resolution", "480p").Error; err != nil {
			return err
		}
	}
	if err := db.Model(&HappyOysterTravel{}).Where("model = ? AND (creation_model IS NULL OR creation_model = ?)", constant.HappyOysterAdventure, "").Update("creation_model", "simple").Error; err != nil {
		return err
	}
	if err := db.Model(&HappyOysterTravel{}).Where("last_polled_at IS NULL").Update("last_polled_at", 0).Error; err != nil {
		return err
	}
	for _, entry := range []struct {
		table any
		index string
	}{
		{&HappyOysterWorld{}, "idx_happy_oyster_world_user"},
		{&HappyOysterTravel{}, "idx_happy_oyster_travel_user"},
	} {
		if db.Migrator().HasIndex(entry.table, entry.index) {
			if err := db.Migrator().DropIndex(entry.table, entry.index); err != nil {
				return err
			}
		}
	}
	return nil
}
