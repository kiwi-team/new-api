package alerting

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

var ErrTargetKeyUnavailable = errors.New("notification encryption key is unavailable")
var ErrTargetRejected = errors.New("notification target is not allowed")

// Validation errors contain static message keys, never destination URLs,
// tokens or resolver errors that might disclose notification credentials.
type TargetValidationError struct {
	Field      string
	MessageKey string
}

func (e *TargetValidationError) Error() string { return e.MessageKey }
func (e *TargetValidationError) Unwrap() error { return ErrTargetRejected }

type DeliveryError struct{ RetryAfter time.Duration }

func (e *DeliveryError) Error() string { return "notification rejected" }

type TargetSecret struct {
	PrivateNetwork bool   `json:"private_network,omitempty"`
	Address        string `json:"address"`
	Secret         string `json:"secret"`
	Token          string `json:"token"`
	Priority       int    `json:"priority"`
}

type platformTargetContextKey struct{}

func targetIPAllowed(ctx context.Context, host string, ip net.IP) bool {
	protection := common.SSRFProtection{AllowPrivateIp: false}
	if protection.ValidateResolvedIP(host, ip) == nil {
		return true
	}
	allowed, _ := ctx.Value(platformTargetContextKey{}).(bool)
	if !allowed || !ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	for value := range strings.SplitSeq(os.Getenv("ALERTING_PRIVATE_TARGET_CIDRS"), ",") {
		_, network, err := net.ParseCIDR(strings.TrimSpace(value))
		if err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

type TargetInput struct {
	Name             string        `json:"name"`
	Type             string        `json:"type"`
	Enabled          bool          `json:"enabled"`
	Language         string        `json:"language"`
	ExpectedRevision int64         `json:"expected_revision"`
	Credentials      *TargetSecret `json:"credentials,omitempty"`
	CredentialAction string        `json:"credential_action,omitempty"`
}

type TargetVault struct {
	active string
	keys   map[string][]byte
	Client *http.Client
}

// NewTargetVault accepts a deployment-owned key ring, allowing rotation while
// existing targets remain decryptable. A missing key never means plaintext.
func NewTargetVault(active string, keys map[string]string) (*TargetVault, error) {
	v := &TargetVault{active: active, keys: make(map[string][]byte), Client: NewTargetHTTPClient()}
	for id, encoded := range keys {
		key, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(key) != 32 || len(id) == 0 || len(id) > 32 {
			return nil, ErrTargetKeyUnavailable
		}
		v.keys[id] = key
	}
	if len(v.keys[active]) != 32 {
		return nil, ErrTargetKeyUnavailable
	}
	return v, nil
}

func (v *TargetVault) Seal(owner int, kind string, secret TargetSecret) (string, error) {
	if v == nil {
		return "", ErrTargetKeyUnavailable
	}
	block, err := aes.NewCipher(v.keys[v.active])
	if err != nil {
		return "", ErrTargetKeyUnavailable
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	plain, err := common.Marshal(secret)
	if err != nil {
		return "", err
	}
	associated := []byte(fmt.Sprintf("new-api:alert-target:v1:%d:%s", owner, kind))
	return base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, plain, associated)), nil
}

func (v *TargetVault) Open(target model.AlertTarget) (TargetSecret, error) {
	var secret TargetSecret
	if v == nil || len(v.keys[target.KeyID]) != 32 {
		return secret, ErrTargetKeyUnavailable
	}
	block, err := aes.NewCipher(v.keys[target.KeyID])
	if err != nil {
		return secret, ErrTargetKeyUnavailable
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return secret, err
	}
	data, err := base64.StdEncoding.DecodeString(target.Ciphertext)
	if err != nil || len(data) < aead.NonceSize() {
		return secret, ErrTargetKeyUnavailable
	}
	plain, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], []byte(fmt.Sprintf("new-api:alert-target:v1:%d:%s", target.OwnerUserID, target.Type)))
	if err != nil {
		return secret, ErrTargetKeyUnavailable
	}
	if common.Unmarshal(plain, &secret) != nil {
		return secret, ErrTargetKeyUnavailable
	}
	return secret, nil
}

func ValidateTargetAddress(ctx context.Context, kind string, secret TargetSecret) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if kind == "email" {
		address, err := mail.ParseAddress(secret.Address)
		if err != nil || address.Address != secret.Address || strings.ContainsAny(secret.Address, "\r\n,") {
			return "", &TargetValidationError{Field: "credentials.address", MessageKey: "Enter a valid notification email address."}
		}
		local, domain, _ := strings.Cut(address.Address, "@")
		return local[:1] + "***@" + domain, nil
	}
	if !slices.Contains([]string{"feishu", "wecom", "webhook_v1", "webhook_v2", "bark", "gotify"}, kind) {
		return "", &TargetValidationError{Field: "type", MessageKey: "Unsupported notification target type."}
	}
	u, err := url.Parse(secret.Address)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" || u.Fragment != "" || len(secret.Address) > 4096 || (u.Port() != "" && u.Port() != "443") {
		return "", &TargetValidationError{Field: "credentials.address", MessageKey: "Notification URL must use HTTPS on port 443 without a username, password or fragment."}
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, u.Hostname())
	if err != nil || len(ips) == 0 {
		return "", &TargetValidationError{Field: "credentials.address", MessageKey: "Notification hostname could not be resolved."}
	}
	for _, ip := range ips {
		if !targetIPAllowed(ctx, u.Hostname(), ip.IP) {
			return "", &TargetValidationError{Field: "credentials.address", MessageKey: "Notification destination is blocked by network access rules."}
		}
	}
	if kind == "gotify" && secret.Token == "" {
		return "", &TargetValidationError{Field: "credentials.token", MessageKey: "Gotify requires an application token."}
	}
	return "https://" + u.Hostname() + "/…", nil
}

// Dial-time validation pins a verified address for this connection. It remains
// strict even when the application's unrelated fetch policy allows private IPs.
func NewTargetHTTPClient() *http.Client {
	transport := &http.Transport{MaxIdleConns: 20, MaxIdleConnsPerHost: 2, IdleConnTimeout: time.Minute, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || port != "443" {
			return nil, ErrTargetRejected
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(ips) == 0 {
			return nil, ErrTargetRejected
		}
		for _, ip := range ips {
			if !targetIPAllowed(ctx, host, ip.IP) {
				return nil, ErrTargetRejected
			}
		}
		dialer := net.Dialer{Timeout: 5 * time.Second}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
	return &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (v *TargetVault) Save(ctx context.Context, actor Actor, owner int, id int64, input TargetInput) (*model.AlertTarget, error) {
	if actor.ID <= 0 || owner != actor.ID && !actor.Root() {
		return nil, ErrForbidden
	}
	if owner < 0 {
		return nil, ErrInvalid
	}
	if owner > 0 && owner != actor.ID {
		var user model.User
		if err := model.DB.WithContext(ctx).Select("id").First(&user, owner).Error; err != nil {
			return nil, err
		}
	}
	if len(strings.TrimSpace(input.Name)) == 0 || len(input.Name) > 160 {
		return nil, ErrInvalid
	}
	if !slices.Contains([]string{"", "en", "zh-CN", "zh-TW"}, input.Language) {
		return nil, ErrInvalid
	}
	if input.Credentials != nil && (len(input.Credentials.Secret) > 4096 || len(input.Credentials.Token) > 4096) {
		return nil, ErrInvalid
	}
	var row model.AlertTarget
	if input.CredentialAction != "" && !slices.Contains([]string{"keep", "replace", "clear"}, input.CredentialAction) {
		return nil, ErrInvalid
	}
	if input.CredentialAction == "clear" && (input.Enabled || input.Credentials != nil) || input.CredentialAction == "replace" && input.Credentials == nil || input.CredentialAction == "keep" && input.Credentials != nil {
		return nil, ErrInvalid
	}
	if id != 0 {
		if err := model.DB.WithContext(ctx).First(&row, id).Error; err != nil {
			return nil, err
		}
		if row.OwnerUserID != owner || row.Revision != input.ExpectedRevision {
			return nil, model.ErrAlertConflict
		}
		if row.Type != input.Type {
			return nil, ErrInvalid
		}
	} else if input.Credentials == nil {
		return nil, ErrInvalid
	}
	if input.CredentialAction == "clear" {
		row.Ciphertext, row.KeyID, row.RateKey, row.AddressHint = "", "", "", ""
	}
	if input.Credentials == nil && row.Ciphertext != "" && row.KeyID != v.active {
		// A normal authorized save rewraps old ciphertext without exposing it
		// to the caller. Keep the old key until all targets have been rotated.
		secret, err := v.Open(row)
		if err != nil {
			return nil, err
		}
		sealed, err := v.Seal(owner, row.Type, secret)
		if err != nil {
			return nil, err
		}
		row.Ciphertext, row.KeyID = sealed, v.active
	}
	if input.Credentials != nil {
		if input.Credentials.PrivateNetwork {
			if owner != 0 || !actor.Root() {
				return nil, ErrForbidden
			}
			ctx = context.WithValue(ctx, platformTargetContextKey{}, true)
		}
		hint, err := ValidateTargetAddress(ctx, input.Type, *input.Credentials)
		if err != nil {
			return nil, err
		}
		text, err := v.Seal(owner, input.Type, *input.Credentials)
		if err != nil {
			return nil, err
		}
		row.AddressHint, row.Ciphertext, row.KeyID = hint, text, v.active
		row.RateKey = model.AlertIdentity("destination", input.Type, input.Credentials.Address, input.Credentials.Token)
	}
	if input.Enabled && row.Ciphertext == "" {
		return nil, ErrTargetRejected
	}
	row.Name, row.Type, row.Enabled, row.Language = input.Name, input.Type, input.Enabled, input.Language
	row.OwnerUserID = owner
	now := time.Now().UnixMilli()
	row.UpdatedAt = now
	row.Revision++
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if id == 0 {
			if err := model.LockAlertOwner(tx, owner); err != nil {
				return err
			}
			var count int64
			if err := tx.Model(&model.AlertTarget{}).Where("owner_user_id = ?", owner).Count(&count).Error; err != nil {
				return err
			}
			if count >= int64(model.AlertTargetOwnerLimit()) {
				return ErrInvalid
			}
			row.CreatedAt = now
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		} else {
			result := tx.Model(&model.AlertTarget{}).Where("id = ? AND revision = ? AND owner_user_id = ?", id, input.ExpectedRevision, owner).
				Select("name", "enabled", "language", "address_hint", "rate_key", "ciphertext", "key_id", "revision", "updated_at").Updates(&row)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return model.ErrAlertConflict
			}
		}
		return tx.Create(&model.AlertAudit{ActorID: actor.ID, OwnerUserID: owner, Operation: "target.save", ObjectID: row.ID, Revision: row.Revision, CreatedAt: now}).Error
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (v *TargetVault) Deliver(ctx context.Context, target model.AlertTarget, event model.AlertEvent) (int, error) {
	secret, err := v.Open(target)
	if err != nil {
		return 0, err
	}
	if target.OwnerUserID == 0 && secret.PrivateNetwork {
		ctx = context.WithValue(ctx, platformTargetContextKey{}, true)
	}
	if _, err := ValidateTargetAddress(ctx, target.Type, secret); err != nil {
		return 0, err
	}
	title, message := "new-api alert", RenderNotification(target.Language, event)
	if event.Kind == "legacy_notice" {
		var notice LegacyNotice
		if common.UnmarshalJsonStr(event.Evidence, &notice) != nil {
			return 0, ErrInvalid
		}
		title, message = notice.Title, notice.Content
	}
	if target.Type == "email" {
		return 0, common.SendEmailContext(ctx, title, secret.Address, html.EscapeString(message))
	}
	event.Summary = RenderNotification(target.Language, event)
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	payload := map[string]any{}
	switch target.Type {
	case "feishu":
		payload = map[string]any{"msg_type": "text", "content": map[string]string{"text": html.EscapeString(event.Summary)}}
		if secret.Secret != "" {
			mac := hmac.New(sha256.New, []byte(stamp+"\n"+secret.Secret))
			payload["timestamp"] = stamp
			payload["sign"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
		}
	case "wecom":
		payload = map[string]any{"msgtype": "text", "text": map[string]string{"content": event.Summary}}
	case "bark":
		payload = map[string]any{"title": title, "body": message}
	case "gotify":
		payload = map[string]any{"title": title, "message": message, "priority": max(1, min(secret.Priority, 10))}
	case "webhook_v1":
		payload = map[string]any{"type": "alert", "title": "new-api alert", "content": event.Summary, "timestamp": time.Now().Unix()}
		if event.Kind == "legacy_notice" {
			var notice LegacyNotice
			if common.UnmarshalJsonStr(event.Evidence, &notice) != nil {
				return 0, ErrInvalid
			}
			payload["type"], payload["title"], payload["content"] = notice.Type, notice.Title, notice.Content
			if len(notice.Values) > 0 {
				payload["values"] = notice.Values
			}
		}
	case "webhook_v2":
		payload = map[string]any{"version": 2, "event_id": event.ID, "kind": event.Kind, "summary": event.Summary, "created_at": event.CreatedAt}
	default:
		return 0, ErrTargetRejected
	}
	body, err := common.Marshal(payload)
	if err != nil {
		return 0, err
	}
	if target.Type == "feishu" && len(body) > 20*1024 {
		return 0, ErrTargetRejected
	}
	address := secret.Address
	if target.Type == "gotify" {
		u, _ := url.Parse(address)
		if !strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/message") {
			u.Path = strings.TrimRight(u.Path, "/") + "/message"
		}
		address = u.String()
	}
	method := http.MethodPost
	var requestBody io.Reader = bytes.NewReader(body)
	if target.Type == "bark" && (strings.Contains(address, "{{title}}") || strings.Contains(address, "{{content}}")) {
		// Preserve the legacy Bark URL-template contract without forwarding
		// unescaped names into the URL or allowing a host/scheme substitution.
		u, _ := url.Parse(address)
		if strings.Contains(u.Host, "{{") {
			return 0, ErrTargetRejected
		}
		address = strings.ReplaceAll(address, "{{title}}", url.QueryEscape(title))
		address = strings.ReplaceAll(address, "{{content}}", url.QueryEscape(message))
		if len(address) > 8192 {
			return 0, ErrTargetRejected
		}
		if _, err := ValidateTargetAddress(ctx, target.Type, TargetSecret{Address: address}); err != nil {
			return 0, err
		}
		method, requestBody = http.MethodGet, nil
	}
	req, err := http.NewRequestWithContext(ctx, method, address, requestBody)
	if err != nil {
		return 0, ErrTargetRejected
	}
	req.Header.Set("Content-Type", "application/json")
	if target.Type == "gotify" {
		req.Header.Set("X-Gotify-Key", secret.Token)
	}
	if strings.HasPrefix(target.Type, "webhook_") && secret.Secret != "" {
		mac := hmac.New(sha256.New, []byte(secret.Secret))
		if target.Type == "webhook_v2" {
			delivery := model.AlertIdentity(event.ID, target.ID, target.Revision)
			req.Header.Set("X-NewAPI-Event-ID", strconv.FormatInt(event.ID, 10))
			req.Header.Set("X-NewAPI-Delivery-ID", delivery)
			req.Header.Set("X-NewAPI-Timestamp", stamp)
			_, _ = mac.Write([]byte(stamp + "." + delivery + "."))
			_, _ = mac.Write(body)
			req.Header.Set("X-NewAPI-Signature", hex.EncodeToString(mac.Sum(nil)))
		} else {
			_, _ = mac.Write(body)
			req.Header.Set("X-Webhook-Signature", hex.EncodeToString(mac.Sum(nil)))
		}
	}
	response, err := v.Client.Do(req)
	if err != nil {
		return 0, errors.New("notification transport failed")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(data) > 65536 {
		return response.StatusCode, errors.New("notification response invalid")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		delay := time.Duration(0)
		if seconds, err := strconv.ParseInt(response.Header.Get("Retry-After"), 10, 64); err == nil {
			delay = time.Duration(min(max(seconds, 0), 3600)) * time.Second
		} else if deadline, err := http.ParseTime(response.Header.Get("Retry-After")); err == nil {
			delay = min(max(time.Until(deadline), time.Duration(0)), time.Hour)
		}
		return response.StatusCode, &DeliveryError{RetryAfter: delay}
	}
	if target.Type == "feishu" || target.Type == "wecom" || target.Type == "bark" {
		var result struct {
			Code       *int `json:"code"`
			ErrCode    *int `json:"errcode"`
			StatusCode *int `json:"StatusCode"`
		}
		if common.Unmarshal(data, &result) != nil {
			return response.StatusCode, errors.New("notification response invalid")
		}
		code := result.Code
		if target.Type == "wecom" {
			code = result.ErrCode
		}
		if code == nil {
			code = result.StatusCode
		}
		if code == nil || *code != 0 && !(target.Type == "bark" && *code == 200) {
			return response.StatusCode, errors.New("notification platform rejected")
		}
	}
	return response.StatusCode, nil
}
