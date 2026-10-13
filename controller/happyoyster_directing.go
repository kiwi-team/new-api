package controller

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type happyOysterImage struct {
	URL    string `json:"url,omitempty"`
	Base64 string `json:"base64,omitempty"`
}

type happyOysterAct struct {
	Turn       *int   `json:"turn,omitempty"`
	Content    string `json:"content"`
	CameraType string `json:"cameraType,omitempty"`
	ShotSize   string `json:"shotSize,omitempty"`
	Cut        string `json:"cut,omitempty"`
}

type happyOysterScript struct {
	Synopsis   string   `json:"synopsis"`
	VideoTitle string   `json:"videoTitle,omitempty"`
	Scene      string   `json:"scene,omitempty"`
	Style      string   `json:"style,omitempty"`
	Speed      string   `json:"speed,omitempty"`
	Language   string   `json:"language,omitempty"`
	Setting    string   `json:"setting,omitempty"`
	Soundtrack string   `json:"soundtrack,omitempty"`
	Prologue   string   `json:"prologue,omitempty"`
	VideoTags  []string `json:"videoTags,omitempty"`
	Subjects   []struct {
		Label      string            `json:"label"`
		Name       string            `json:"name"`
		Type       string            `json:"type"`
		RefImage   *happyOysterImage `json:"refImage,omitempty"`
		Gender     string            `json:"gender,omitempty"`
		Position   string            `json:"position,omitempty"`
		Ethnicity  string            `json:"ethnicity,omitempty"`
		Age        string            `json:"age,omitempty"`
		Appearance string            `json:"appearance,omitempty"`
		Voice      string            `json:"voice,omitempty"`
	} `json:"subjects,omitempty"`
	Acts []happyOysterAct `json:"acts"`
}

type happyOysterCreateRequest struct {
	Async           *bool              `json:"async,omitempty"`
	Mode            *int               `json:"mode,omitempty"`
	Perspective     string             `json:"perspective,omitempty"`
	Prompt          string             `json:"prompt,omitempty"`
	CreationModel   string             `json:"creationModel,omitempty"`
	UploadMode      string             `json:"uploadMode,omitempty"`
	RefWorldId      string             `json:"refWorldId,omitempty"`
	Resolution      string             `json:"resolution"`
	EventStyle      string             `json:"eventStyle,omitempty"`
	Layout          string             `json:"layout,omitempty"`
	Narrative       string             `json:"narrative,omitempty"`
	FirstFrameImage happyOysterImage   `json:"firstFrameImage,omitempty"`
	InputImages     []happyOysterImage `json:"inputImages,omitempty"`
	ScriptList      *happyOysterScript `json:"scriptList,omitempty"`
}

func (r happyOysterCreateRequest) hasReferenceImage() bool {
	if r.FirstFrameImage.URL != "" || r.FirstFrameImage.Base64 != "" || len(r.InputImages) > 0 {
		return true
	}
	if r.ScriptList != nil {
		for _, subject := range r.ScriptList.Subjects {
			if subject.RefImage != nil {
				return true
			}
		}
	}
	return false
}

func (r happyOysterCreateRequest) validateDirecting() error {
	if r.Mode != nil && *r.Mode != 2 {
		return fmt.Errorf("mode must be 2 for HappyOyster Directing")
	}
	if r.Resolution != "480p" && r.Resolution != "720p" {
		return fmt.Errorf("resolution must be 480p or 720p")
	}
	if len(r.RefWorldId) > 512 {
		return fmt.Errorf("refWorldId is too long")
	}
	if r.CreationModel != "" && r.CreationModel != "simple" && r.CreationModel != "scriptlist" {
		return fmt.Errorf("creationModel must be simple or scriptlist")
	}
	frame := r.FirstFrameImage
	if frame.URL != "" || frame.Base64 != "" {
		if err := frame.validate(true); err != nil {
			return err
		}
	} else if r.CreationModel == "scriptlist" {
		return fmt.Errorf("firstFrameImage is required for scriptlist")
	}
	if r.CreationModel == "scriptlist" {
		if len(r.InputImages) > 0 || r.EventStyle != "" {
			return fmt.Errorf("inputImages and eventStyle are not supported for scriptlist")
		}
		if r.ScriptList == nil {
			return fmt.Errorf("scriptList is required")
		}
		return r.ScriptList.validate(false)
	}
	if strings.TrimSpace(r.Prompt) == "" || utf8.RuneCountInString(r.Prompt) > 2000 {
		return fmt.Errorf("prompt must contain 1 to 2000 characters")
	}
	if !slices.Contains([]string{"", "normal", "dramatic", "regular"}, r.EventStyle) {
		return fmt.Errorf("invalid eventStyle")
	}
	if !slices.Contains([]string{"", "Stable", "Fast", "Calm"}, r.Layout) {
		return fmt.Errorf("invalid layout")
	}
	if !slices.Contains([]string{"", "Calm", "Dramatic", "Normal", "Steady"}, r.Narrative) {
		return fmt.Errorf("invalid narrative")
	}
	if len(r.InputImages) > 6 {
		return fmt.Errorf("inputImages must contain at most 6 images")
	}
	for _, img := range r.InputImages {
		if err := img.validate(true); err != nil {
			return err
		}
	}
	return nil
}

func (img happyOysterImage) validate(landscape bool) error {
	if (img.URL == "") == (img.Base64 == "") {
		return fmt.Errorf("image must contain exactly one of url or base64")
	}
	return validateHappyOysterImage(img.URL, img.Base64, landscape)
}

var happyOysterSubjectLabel = regexp.MustCompile(`^\[character_[A-Za-z0-9_]+\]$`)
var happyOysterClientRequestID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

func (s happyOysterScript) validate(update bool) error {
	if !update {
		if strings.TrimSpace(s.Synopsis) == "" || utf8.RuneCountInString(s.Synopsis) > 2000 {
			return fmt.Errorf("synopsis must contain 1 to 2000 characters")
		}
		for _, field := range []struct {
			value string
			limit int
		}{
			{s.VideoTitle, 128}, {s.Scene, 64}, {s.Style, 64}, {s.Speed, 64}, {s.Language, 64}, {s.Setting, 2000}, {s.Soundtrack, 500}, {s.Prologue, 1000},
		} {
			if utf8.RuneCountInString(field.value) > field.limit {
				return fmt.Errorf("scriptList metadata exceeds its character limit")
			}
		}
		if len(s.VideoTags) > 20 || len(s.Subjects) > 6 {
			return fmt.Errorf("scriptList supports at most 20 tags and 6 subjects")
		}
		for _, tag := range s.VideoTags {
			if utf8.RuneCountInString(tag) > 32 {
				return fmt.Errorf("video tag exceeds 32 characters")
			}
		}
		for _, subject := range s.Subjects {
			if subject.Label != "" && !happyOysterSubjectLabel.MatchString(subject.Label) || !slices.Contains([]string{"", "character", "animal", "creature", "narrator"}, subject.Type) {
				return fmt.Errorf("invalid subject label or type")
			}
			for _, field := range []struct {
				value string
				limit int
			}{
				{subject.Name, 64}, {subject.Gender, 64}, {subject.Position, 64}, {subject.Ethnicity, 64}, {subject.Age, 64}, {subject.Appearance, 500}, {subject.Voice, 200},
			} {
				if utf8.RuneCountInString(field.value) > field.limit {
					return fmt.Errorf("subject metadata exceeds its character limit")
				}
			}
			if subject.RefImage != nil {
				if err := subject.RefImage.validate(false); err != nil {
					return err
				}
			}
		}
	}
	if len(s.Acts) < 1 || len(s.Acts) > 45 || update && len(s.Acts) != 45 {
		return fmt.Errorf("scriptList.acts must contain 1 to 45 acts (exactly 45 for update-script)")
	}
	turns := make(map[int]bool, len(s.Acts))
	total := 0
	for i, act := range s.Acts {
		turn := i + 1
		if act.Turn != nil {
			turn = *act.Turn
		} else if update {
			return fmt.Errorf("turn is required for update-script")
		}
		if turn < 1 || turn > 45 || turns[turn] {
			return fmt.Errorf("act turns must be unique integers between 1 and 45")
		}
		turns[turn] = true
		length := utf8.RuneCountInString(act.Content)
		if strings.TrimSpace(act.Content) == "" || length > 2000 {
			return fmt.Errorf("act content must contain 1 to 2000 characters")
		}
		total += length
		if !slices.Contains([]string{"", "Static", "Tracking", "Pan Left", "Pan Right", "Tilt Up", "Tilt Down", "Push-in", "Pull-out", "POV Forward", "POV Look Down", "POV Look Up", "POV Turn Left", "POV Turn Right"}, act.CameraType) {
			return fmt.Errorf("invalid cameraType")
		}
		if !slices.Contains([]string{"", "Wide", "Medium", "Close-up"}, act.ShotSize) {
			return fmt.Errorf("invalid shotSize")
		}
		if !slices.Contains([]string{"", "long-take", "hard-cut", "cut-in", "cut-out", "cutaway", "cutback", "camera movement transition"}, act.Cut) {
			return fmt.Errorf("invalid cut")
		}
	}
	if total > 100000 {
		return fmt.Errorf("total act content exceeds 100000 characters")
	}
	return nil
}

type happyOysterControlRequest struct {
	EncryptedTravelId string   `json:"encryptedTravelId"`
	UserAgent         *string  `json:"userAgent,omitempty"`
	RewindToSec       *float64 `json:"rewindToSec,omitempty"`
	Content           string   `json:"content,omitempty"`
	ClientRequestId   string   `json:"clientRequestId,omitempty"`
	ScriptList        *struct {
		Acts []happyOysterAct `json:"acts"`
	} `json:"scriptList,omitempty"`
	FailCode string `json:"failCode,omitempty"`
}

func (r happyOysterControlRequest) validate(path string) error {
	if r.UserAgent != nil && strings.TrimSpace(*r.UserAgent) == "" {
		return fmt.Errorf("userAgent must not be empty")
	}
	switch path {
	case "/travels/rewind":
		if r.RewindToSec == nil || *r.RewindToSec < 0 {
			return fmt.Errorf("rewindToSec must be a non-negative number")
		}
	case "/travels/instruct":
		if strings.TrimSpace(r.Content) == "" || utf8.RuneCountInString(r.Content) > 2000 {
			return fmt.Errorf("content must contain 1 to 2000 characters")
		}
	case "/travels/update-script":
		if r.ScriptList == nil {
			return fmt.Errorf("scriptList is required")
		}
		return (happyOysterScript{Acts: r.ScriptList.Acts}).validate(true)
	case "/travels/end":
		if r.FailCode != "" && r.FailCode != "TRAVEL_NO_STREAM_AUTO_END" {
			return fmt.Errorf("invalid failCode")
		}
	}
	return nil
}

// Each price dimension is exclusive: a world is created once, an instruction
// is charged once, and only the original video's selected resolution is billed.
func happyOysterDirectingUsage(resolution string, reference bool, worlds, seconds, instructions int) map[string]any {
	facts := map[string]any{"world_creations_without_reference": 0, "world_creations_with_reference": 0, "instruction_injections": min(max(instructions, 0), 1), "experience_480p_seconds": 0, "experience_720p_seconds": 0}
	worldKey := "world_creations_without_reference"
	if reference {
		worldKey = "world_creations_with_reference"
	}
	facts[worldKey] = min(max(worlds, 0), 1)
	if resolution == "720p" {
		facts["experience_720p_seconds"] = min(max(seconds, 0), 180)
	} else {
		facts["experience_480p_seconds"] = min(max(seconds, 0), 180)
	}
	return facts
}

func happyOysterInstruct(c *gin.Context, travel *model.HappyOysterTravel, channel *model.Channel, key string, body []byte) {
	var req happyOysterControlRequest
	if err := common.Unmarshal(body, &req); err != nil {
		happyOysterBusinessError(c, 400000, "invalid JSON body")
		return
	}
	if !happyOysterClientRequestID.MatchString(req.ClientRequestId) {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			happyOysterError(c, 500, "could not generate clientRequestId")
			return
		}
		req.ClientRequestId = hex.EncodeToString(id[:])
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(body, &fields); err != nil {
		happyOysterBusinessError(c, 400000, "invalid JSON body")
		return
	}
	fields["clientRequestId"], _ = common.Marshal(req.ClientRequestId)
	body, _ = common.Marshal(fields)
	hash := sha256.Sum256([]byte(req.Content))
	// Hash the case-sensitive protocol ID so MySQL's default collation cannot
	// conflate distinct IDs such as "turnA" and "turna".
	requestKey := sha256.Sum256([]byte(req.ClientRequestId))
	record := model.HappyOysterInstruction{TravelId: travel.Id, ClientRequestId: req.ClientRequestId, RequestKey: hex.EncodeToString(requestKey[:]), RequestHash: hex.EncodeToString(hash[:]), State: "pending"}
	// A plain unique insert also works when MySQL enables clientFoundRows;
	// an upsert's RowsAffected cannot reliably distinguish the winning request.
	if err := model.DB.Create(&record).Error; err != nil {
		var existing model.HappyOysterInstruction
		if err := model.DB.Where("travel_id = ? AND request_key = ?", travel.Id, record.RequestKey).First(&existing).Error; err != nil {
			happyOysterError(c, 500, "could not read instruction")
			return
		}
		if existing.RequestHash != record.RequestHash || existing.State != "done" {
			happyOysterError(c, 409, "clientRequestId conflicts or its previous outcome is still unknown")
			return
		}
		c.Data(existing.HTTPStatus, "application/json", []byte(existing.Response))
		return
	}
	info, err := happyOysterRelayInfo(c)
	if err != nil {
		model.DB.Delete(&record)
		happyOysterError(c, 500, err.Error())
		return
	}
	c.Set("happyoyster_instructions", 1)
	quota, err := happyOysterPrice(c, info, channel.Other, travel.BaseURL, 0, 0)
	if err != nil {
		model.DB.Delete(&record)
		happyOysterError(c, 400, err.Error())
		return
	}
	if apiErr := service.PreConsumeBilling(c, quota, info); apiErr != nil {
		// No upstream action happened, so this reservation can safely be retried.
		model.DB.Delete(&record)
		happyOysterError(c, apiErr.StatusCode, apiErr.Error())
		return
	}
	snapshot, marshalErr := common.Marshal(info.TieredBillingSnapshot)
	if marshalErr != nil {
		info.Billing.Refund(c)
		happyOysterError(c, 500, "could not persist instruction billing")
		return
	}
	if err := model.DB.Model(&record).Updates(map[string]any{"token_id": info.TokenId, "pre_consumed_quota": info.FinalPreConsumedQuota, "billing_source": info.BillingSource, "subscription_id": info.SubscriptionId, "billing_snapshot": string(snapshot)}).Error; err != nil {
		info.Billing.Refund(c)
		happyOysterError(c, 500, "could not persist instruction billing")
		return
	}
	status, payload, envelope, err := happyOysterProxy(c, channel, key, http.MethodPost, "/travels/instruct", nil, body)
	var accepted struct {
		Accepted          bool   `json:"accepted"`
		EncryptedTravelId string `json:"encryptedTravelId"`
	}
	invalidSuccess := envelope.successful() && (common.Unmarshal(envelope.Data, &accepted) != nil || !accepted.Accepted || accepted.EncryptedTravelId != "" && accepted.EncryptedTravelId != travel.EncryptedTravelId)
	if err != nil || status >= 500 || invalidSuccess {
		// Upstream may have executed: retain the reservation and pending key,
		// never blindly replay or refund a possibly billable action.
		_, _ = happyOysterLog(c, info, info.FinalPreConsumedQuota, "HappyOyster instruction outcome unknown; reconciliation required", travel.EncryptedWorldId, travel.EncryptedTravelId, happyOysterLogPayload{})
		happyOysterError(c, 502, "instruction outcome unknown; do not retry with a new clientRequestId")
		return
	}
	actual := quota
	if status < 200 || status >= 300 || !envelope.successful() {
		actual = 0
	}
	if err := service.SettleBilling(c, info, actual); err != nil {
		happyOysterError(c, 500, "instruction settlement requires reconciliation")
		return
	}
	if actual > 0 {
		_, _ = happyOysterLog(c, info, actual, "HappyOyster instruction injection", travel.EncryptedWorldId, travel.EncryptedTravelId, happyOysterLogPayload{})
	}
	if err := model.DB.Model(&record).Updates(map[string]any{"state": "done", "http_status": status, "response": string(payload)}).Error; err != nil {
		happyOysterError(c, 500, "instruction outcome persistence failed")
		return
	}
	c.Data(status, "application/json", payload)
}
