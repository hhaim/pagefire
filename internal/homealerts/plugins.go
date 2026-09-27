package homealerts

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Provider interface {
	Kind() string
	Name() string
	DestinationLabel() string
	SecretLabel() string
	Validate(destination, secret string) error
	Send(ctx context.Context, delivery Notification) error
}

type Notification struct {
	Destination           string
	Secret                string
	Message               string
	AckAlertID            string
	RepeatIntervalSeconds int
	ExpireSeconds         int
}

type ActionProvider interface {
	Poll(ctx context.Context, destination, secret string, acknowledge func(context.Context, string, string) error) error
}

type actionResetter interface{ ResetActions() }

type Plugin struct {
	Kind             string `json:"kind"`
	Name             string `json:"name"`
	DestinationLabel string `json:"destination_label"`
	SecretLabel      string `json:"secret_label"`
	Destination      string `json:"destination"`
	Enabled          bool   `json:"enabled"`
	Configured       bool   `json:"configured"`
}

type PluginInput struct {
	Destination string `json:"destination"`
	Secret      string `json:"secret"`
	Enabled     bool   `json:"enabled"`
}

func (s *Service) PluginKinds() []string {
	kinds := make([]string, 0, len(s.providers))
	for kind := range s.providers {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

func (s *Service) Plugins(ctx context.Context) ([]Plugin, error) {
	plugins := []Plugin{}
	for _, kind := range s.PluginKinds() {
		provider := s.providers[kind]
		p := Plugin{Kind: kind, Name: provider.Name(), DestinationLabel: provider.DestinationLabel(), SecretLabel: provider.SecretLabel()}
		var enabled int
		err := s.db.QueryRowContext(ctx, `SELECT destination,enabled FROM home_plugins WHERE kind=?`, kind).Scan(&p.Destination, &enabled)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if err == nil {
			p.Enabled, p.Configured = enabled != 0, true
		}
		plugins = append(plugins, p)
	}
	return plugins, nil
}

func (s *Service) PutPlugin(ctx context.Context, kind string, input PluginInput) (Plugin, error) {
	p, ok := s.providers[kind]
	if !ok {
		return Plugin{}, fmt.Errorf("%w: unknown plugin", ErrInvalid)
	}
	input.Destination = strings.TrimSpace(input.Destination)
	secret := input.Secret
	if secret == "" {
		var encrypted string
		err := s.db.QueryRowContext(ctx, `SELECT secret FROM home_plugins WHERE kind=?`, kind).Scan(&encrypted)
		if err != nil {
			return Plugin{}, fmt.Errorf("%w: secret is required", ErrInvalid)
		}
		var err2 error
		secret, err2 = s.decrypt(encrypted)
		if err2 != nil {
			return Plugin{}, err2
		}
	}
	if err := p.Validate(input.Destination, secret); err != nil {
		return Plugin{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	encrypted, err := s.encrypt(secret)
	if err != nil {
		return Plugin{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO home_plugins(kind,destination,secret,enabled,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(kind) DO UPDATE SET destination=excluded.destination,secret=excluded.secret,enabled=excluded.enabled,updated_at=excluded.updated_at`, kind, input.Destination, encrypted, input.Enabled, time.Now().UTC().Unix())
	if err != nil {
		return Plugin{}, err
	}
	if resetter, ok := p.(actionResetter); ok {
		resetter.ResetActions()
	}
	return Plugin{Kind: kind, Name: p.Name(), DestinationLabel: p.DestinationLabel(), SecretLabel: p.SecretLabel(), Destination: input.Destination, Enabled: input.Enabled, Configured: true}, nil
}

func (s *Service) DeletePlugin(ctx context.Context, kind string) error {
	if _, ok := s.providers[kind]; !ok {
		return ErrNotFound
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM home_plugins WHERE kind=?`, kind)
	if resetter, ok := s.providers[kind].(actionResetter); ok {
		resetter.ResetActions()
	}
	return err
}

func (s *Service) TestPlugin(ctx context.Context, kind string) error {
	p, ok := s.providers[kind]
	if !ok {
		return ErrNotFound
	}
	var destination, encrypted string
	if err := s.db.QueryRowContext(ctx, `SELECT destination,secret FROM home_plugins WHERE kind=?`, kind).Scan(&destination, &encrypted); err != nil {
		return ErrNotFound
	}
	secret, err := s.decrypt(encrypted)
	if err != nil {
		return err
	}
	return p.Send(ctx, Notification{Destination: destination, Secret: secret, Message: "PageFire test notification"})
}

func (s *Service) encrypt(plain string) (string, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func (s *Service) decrypt(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("invalid plugin secret")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func loadKey(dataDir string) ([]byte, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "home-alerts.key")
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != 32 {
			return nil, fmt.Errorf("invalid home alert key at %s", path)
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		key, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("invalid home alert key at %s", path)
		}
		return key, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Write(key); err != nil {
		return nil, err
	}
	return key, nil
}

type telegramProvider struct {
	client *http.Client
	offset int64
	mu     sync.Mutex
}

func (p *telegramProvider) ResetActions() { p.mu.Lock(); p.offset = 0; p.mu.Unlock() }

func NewTelegram() Provider {
	return &telegramProvider{client: &http.Client{Timeout: 10 * time.Second}}
}
func (*telegramProvider) Kind() string             { return "telegram" }
func (*telegramProvider) Name() string             { return "Telegram" }
func (*telegramProvider) DestinationLabel() string { return "Chat ID" }
func (*telegramProvider) SecretLabel() string      { return "Bot token" }
func (*telegramProvider) Validate(destination, secret string) error {
	if destination == "" || secret == "" {
		return errors.New("chat ID and bot token are required")
	}
	if _, err := strconv.ParseInt(destination, 10, 64); err != nil {
		return errors.New("Telegram needs a numeric chat ID for acknowledgement buttons")
	}
	if strings.IndexFunc(secret, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ':' || r == '_' || r == '-')
	}) >= 0 {
		return errors.New("invalid bot token")
	}
	return nil
}
func (p *telegramProvider) Send(ctx context.Context, delivery Notification) error {
	payload := map[string]any{"chat_id": delivery.Destination, "text": truncate(delivery.Message, 4096)}
	if delivery.AckAlertID != "" {
		payload["reply_markup"] = map[string]any{"inline_keyboard": [][]map[string]string{{{"text": "Acknowledge", "callback_data": "ack:" + delivery.AckAlertID}}}}
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+delivery.Secret+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return errors.New("telegram request failed")
	}
	defer resp.Body.Close()
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result); err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK || !result.OK {
		return fmt.Errorf("telegram: %s", result.Description)
	}
	return nil
}

func (p *telegramProvider) Poll(ctx context.Context, destination, secret string, acknowledge func(context.Context, string, string) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	endpoint := "https://api.telegram.org/bot" + secret + "/getUpdates?timeout=0&allowed_updates=%5B%22callback_query%22%5D&offset=" + strconv.FormatInt(p.offset, 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return errors.New("telegram callback poll failed")
	}
	defer resp.Body.Close()
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      []struct {
			UpdateID      int64 `json:"update_id"`
			CallbackQuery struct {
				ID   string `json:"id"`
				Data string `json:"data"`
				From struct {
					ID int64 `json:"id"`
				} `json:"from"`
				Message struct {
					Chat struct {
						ID int64 `json:"id"`
					} `json:"chat"`
				} `json:"message"`
			} `json:"callback_query"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("telegram callbacks: %s", result.Description)
	}
	chatID, _ := strconv.ParseInt(destination, 10, 64)
	for _, update := range result.Result {
		callback := update.CallbackQuery
		if callback.ID != "" {
			response := "Alert unavailable"
			if callback.Message.Chat.ID == chatID && strings.HasPrefix(callback.Data, "ack:") {
				alertID := strings.TrimPrefix(callback.Data, "ack:")
				err := acknowledge(ctx, alertID, "telegram:"+strconv.FormatInt(callback.From.ID, 10))
				if err == nil {
					response = "Acknowledged"
				} else if !errors.Is(err, ErrNotFound) {
					return err
				}
			}
			if err := p.answerCallback(ctx, secret, callback.ID, response); err != nil {
				slog.Warn("telegram callback acknowledgement response failed", "error", err)
			}
		}
		p.offset = update.UpdateID + 1
	}
	return nil
}

func (p *telegramProvider) answerCallback(ctx context.Context, secret, id, message string) error {
	body, _ := json.Marshal(map[string]string{"callback_query_id": id, "text": message})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+secret+"/answerCallbackQuery", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return errors.New("telegram callback response failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram callback response: HTTP %d", resp.StatusCode)
	}
	return nil
}

type pushoverProvider struct{ client *http.Client }

func NewPushover() Provider {
	return &pushoverProvider{client: &http.Client{Timeout: 10 * time.Second}}
}
func (*pushoverProvider) Kind() string             { return "pushover" }
func (*pushoverProvider) Name() string             { return "Pushover" }
func (*pushoverProvider) DestinationLabel() string { return "User or group key" }
func (*pushoverProvider) SecretLabel() string      { return "Application token" }
func (*pushoverProvider) Validate(destination, secret string) error {
	if destination == "" || secret == "" {
		return errors.New("user key and application token are required")
	}
	return nil
}
func (p *pushoverProvider) Send(ctx context.Context, delivery Notification) error {
	form := url.Values{"token": {delivery.Secret}, "user": {delivery.Destination}, "message": {truncate(delivery.Message, 1024)}}
	if delivery.AckAlertID != "" {
		form.Set("priority", "1")
	}
	_, err := p.send(ctx, form)
	return err
}

func (p *pushoverProvider) SendEmergency(ctx context.Context, delivery Notification) (string, error) {
	form := url.Values{"token": {delivery.Secret}, "user": {delivery.Destination}, "message": {truncate(delivery.Message, 1024)}}
	form.Set("priority", "2")
	retry := delivery.RepeatIntervalSeconds
	if retry < 30 {
		retry = 30
	}
	if retry > 10800 {
		retry = 10800
	}
	form.Set("retry", strconv.Itoa(retry))
	expire := delivery.ExpireSeconds
	if expire <= 0 || expire > 10800 {
		expire = 10800
	}
	form.Set("expire", strconv.Itoa(expire))
	receipt, err := p.send(ctx, form)
	if err != nil {
		return "", err
	}
	if receipt == "" {
		return "", errors.New("pushover did not return an emergency receipt")
	}
	return receipt, nil
}

func (p *pushoverProvider) send(ctx context.Context, form url.Values) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.pushover.net/1/messages.json", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		Status  int      `json:"status"`
		Errors  []string `json:"errors"`
		Receipt string   `json:"receipt"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result); err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK || result.Status != 1 {
		return "", fmt.Errorf("pushover: %s", strings.Join(result.Errors, ", "))
	}
	return result.Receipt, nil
}

func (p *pushoverProvider) ReceiptStatus(ctx context.Context, secret, receipt string) (bool, int64, error) {
	endpoint := "https://api.pushover.net/1/receipts/" + url.PathEscape(receipt) + ".json?" + url.Values{"token": {secret}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, 0, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false, 0, err
	}
	defer resp.Body.Close()
	var result struct {
		Status         int      `json:"status"`
		Acknowledged   int      `json:"acknowledged"`
		AcknowledgedAt int64    `json:"acknowledged_at"`
		Errors         []string `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result); err != nil {
		return false, 0, err
	}
	if resp.StatusCode != http.StatusOK || result.Status != 1 {
		return false, 0, fmt.Errorf("pushover receipt: %s", strings.Join(result.Errors, ", "))
	}
	return result.Acknowledged == 1, result.AcknowledgedAt, nil
}

func (p *pushoverProvider) CancelReceipt(ctx context.Context, secret, receipt string) error {
	endpoint := "https://api.pushover.net/1/receipts/" + url.PathEscape(receipt) + "/cancel.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(url.Values{"token": {secret}}.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var result struct {
		Status int      `json:"status"`
		Errors []string `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result); err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK || result.Status != 1 {
		return fmt.Errorf("pushover cancel: %s", strings.Join(result.Errors, ", "))
	}
	return nil
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
