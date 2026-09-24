package model

import (
	"time"

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
	Id                   int64  `json:"id"`
	UserId               int    `json:"user_id" gorm:"index;uniqueIndex:idx_happy_oyster_world_user"`
	TokenId              int    `json:"token_id" gorm:"index"`
	EncryptedWorldId     string `json:"encrypted_world_id" gorm:"type:varchar(255);uniqueIndex:idx_happy_oyster_world_user"`
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
	Id                   int64  `json:"id"`
	UserId               int    `json:"user_id" gorm:"index;uniqueIndex:idx_happy_oyster_travel_user"`
	TokenId              int    `json:"token_id" gorm:"index"`
	WorldRecordId        int64  `json:"world_record_id" gorm:"index"`
	EncryptedWorldId     string `json:"encrypted_world_id" gorm:"type:varchar(255);index"`
	EncryptedTravelId    string `json:"encrypted_travel_id" gorm:"type:varchar(255);uniqueIndex:idx_happy_oyster_travel_user"`
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

func GetHappyOysterWorld(userId int, encryptedId string) (*HappyOysterWorld, error) {
	var world HappyOysterWorld
	err := DB.Where("user_id = ? AND encrypted_world_id = ? AND deleted = ?", userId, encryptedId, false).First(&world).Error
	return &world, err
}

func GetDeletedHappyOysterWorld(userId int, encryptedId string) (*HappyOysterWorld, error) {
	var world HappyOysterWorld
	err := DB.Where("user_id = ? AND encrypted_world_id = ? AND deleted = ?", userId, encryptedId, true).First(&world).Error
	return &world, err
}

func GetHappyOysterTravel(userId int, encryptedId string) (*HappyOysterTravel, error) {
	var travel HappyOysterTravel
	err := DB.Where("user_id = ? AND encrypted_travel_id = ?", userId, encryptedId).First(&travel).Error
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
		Updates(map[string]any{"actual_quota": actualQuota, "duration_seconds": duration, "status": status, "settlement_state": HappyOysterSettlementDone, "updated_at": time.Now().Unix()}).Error
}

func RetryHappyOysterSettlement(travelId int64) {
	DB.Model(&HappyOysterTravel{}).
		Where("id = ? AND settlement_state = ?", travelId, HappyOysterSettlementRunning).
		Updates(map[string]any{"settlement_state": HappyOysterSettlementPending, "updated_at": time.Now().Unix()})
}

func ListHappyOysterWorlds(userId, page, pageSize int, status string) ([]HappyOysterWorld, int64, error) {
	var rows []HappyOysterWorld
	var total int64
	query := DB.Model(&HappyOysterWorld{}).Where("user_id = ? AND deleted = ?", userId, false)
	if status == "generating" || status == "ready" || status == "failed" {
		query = query.Where("status = ?", status)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}, Desc: true}).Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error
	return rows, total, err
}

func ListHappyOysterTravels(userId, page, pageSize int, status, encryptedWorldId string) ([]HappyOysterTravel, int64, error) {
	var rows []HappyOysterTravel
	var total int64
	query := DB.Model(&HappyOysterTravel{}).Where("user_id = ?", userId)
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
	DB.Where("settlement_state = ?", HappyOysterSettlementPending).Order("id asc").Limit(limit).Find(&rows)
	return rows
}
