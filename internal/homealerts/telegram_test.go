package homealerts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type rewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	copy.URL.Scheme = t.target.Scheme
	copy.URL.Host = t.target.Host
	copy.Host = t.target.Host
	return t.base.RoundTrip(copy)
}

func TestTelegramInlineAcknowledge(t *testing.T) {
	var button, answered bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			var payload struct {
				ReplyMarkup struct {
					InlineKeyboard [][]struct {
						CallbackData string `json:"callback_data"`
					} `json:"inline_keyboard"`
				} `json:"reply_markup"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			button = len(payload.ReplyMarkup.InlineKeyboard) > 0 && payload.ReplyMarkup.InlineKeyboard[0][0].CallbackData == "ack:alert-1"
			w.Write([]byte(`{"ok":true}`))
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			if r.URL.Query().Get("offset") == "2" {
				w.Write([]byte(`{"ok":true,"result":[]}`))
			} else {
				w.Write([]byte(`{"ok":true,"result":[{"update_id":1,"callback_query":{"id":"callback-1","data":"ack:alert-1","from":{"id":42},"message":{"chat":{"id":1234}}}}]}`))
			}
		case strings.HasSuffix(r.URL.Path, "/answerCallbackQuery"):
			answered = true
			w.Write([]byte(`{"ok":true}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	p := NewTelegram().(*telegramProvider)
	p.client.Transport = rewriteTransport{target: target, base: http.DefaultTransport}
	ctx := context.Background()
	if err := p.Send(ctx, Notification{Destination: "1234", Secret: "123:abc", Message: "High alert", AckAlertID: "alert-1"}); err != nil {
		t.Fatal(err)
	}
	if !button {
		t.Fatal("Telegram high alert had no acknowledge button")
	}
	called := 0
	ack := func(_ context.Context, id, actor string) error {
		called++
		if id != "alert-1" || actor != "telegram:42" {
			t.Errorf("unexpected acknowledgement %q, %q", id, actor)
		}
		return nil
	}
	if err := p.Poll(ctx, "1234", "123:abc", ack); err != nil {
		t.Fatal(err)
	}
	if !answered || called != 1 {
		t.Fatalf("callback answered=%v, acknowledgements=%d", answered, called)
	}
	if err := p.Poll(ctx, "1234", "123:abc", ack); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatal("callback was processed twice")
	}
}
