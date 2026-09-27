package homealerts

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type responseTransport func(*http.Request) (*http.Response, error)

func (f responseTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestUnifiedAlertFeedAndPushoverAck(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newTestService(t)
	var sent []url.Values
	pushover := &pushoverProvider{client: &http.Client{Transport: responseTransport(func(req *http.Request) (*http.Response, error) {
		body := `{"status":1,"acknowledged":1,"acknowledged_at":0}`
		if req.URL.Path == "/1/messages.json" {
			content, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			form, err := url.ParseQuery(string(content))
			if err != nil {
				return nil, err
			}
			sent = append(sent, form)
			body = `{"status":1,"receipt":"receipt-1"}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
	svc.Register(pushover)
	if _, err := svc.PutPlugin(ctx, "pushover", PluginInput{Destination: "user", Secret: "token", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	start, err := svc.Process(ctx, "owner", EventRequest{EventID: "start-1", IncidentKey: "water", Event: "start", Severity: "high", Summary: "Water leak", Details: "Boiler room sensor", RepeatIntervalSeconds: 300})
	if err != nil || start.Status != "applied" {
		t.Fatalf("start: %+v, %v", start, err)
	}
	var status, details string
	if err := db.QueryRowContext(ctx, `SELECT status,details FROM alerts WHERE id=?`, start.AlertID).Scan(&status, &details); err != nil || status != "triggered" || details != "Boiler room sensor" {
		t.Fatalf("canonical alert: status=%q details=%q err=%v", status, details, err)
	}
	info, err := svc.Process(ctx, "owner", EventRequest{EventID: "info-1", Event: "info", Severity: "high", Summary: "Sensor note", Details: "Reading 99"})
	if err != nil || info.Status != "applied" {
		t.Fatalf("info: %+v, %v", info, err)
	}
	if err := svc.dispatchDue(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 2 {
		t.Fatalf("Pushover messages=%d, want 2", len(sent))
	}
	for _, form := range sent {
		if form.Get("priority") != "2" || !strings.Contains(form.Get("message"), "Boiler room sensor") && !strings.Contains(form.Get("message"), "Reading 99") {
			t.Fatalf("high message priority or details missing: %v", form)
		}
	}
	if err := svc.processPushoverReceipts(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status FROM alerts WHERE id=?`, start.AlertID).Scan(&status); err != nil || status != "acknowledged" {
		t.Fatalf("Pushover ACK did not update canonical alert: %q, %v", status, err)
	}
	var nextRepeat any
	if err := db.QueryRowContext(ctx, `SELECT next_repeat_at FROM home_alerts WHERE id=?`, start.AlertID).Scan(&nextRepeat); err != nil || nextRepeat != nil {
		t.Fatalf("repeat not stopped: %v, %v", nextRepeat, err)
	}
	feed, err := svc.Feed(ctx, "owner", FeedFilter{Window: "1h", Type: "start", Open: new(bool)})
	if err != nil {
		t.Fatal(err)
	}
	if feed.Total != 0 {
		t.Fatalf("closed filter included acknowledged start: %d", feed.Total)
	}
	feed, err = svc.Feed(ctx, "owner", FeedFilter{Window: "1h", Type: "start"})
	if err != nil || feed.Total != 1 || feed.Events[0].AlertID != start.AlertID {
		t.Fatalf("start in unified feed: %+v, %v", feed.Events, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO escalation_policies(id,name) VALUES('service-policy','Service policy')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO services(id,name,escalation_policy_id) VALUES('service-1','Service 1','service-policy')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO alerts(id,service_id,status,summary,details,source) VALUES('service-alert-1','service-1','triggered','CPU high','Host A','api')`); err != nil {
		t.Fatal(err)
	}
	feed, err = svc.Feed(ctx, "owner", FeedFilter{Window: "1h"})
	if err != nil || feed.TypeCounts["start"] != 1 || feed.TypeCounts["info"] != 1 || feed.TypeCounts["alert"] != 1 || feed.OpenAlerts != 2 {
		t.Fatalf("combined feed counts: total=%d types=%+v open=%d err=%v", feed.Total, feed.TypeCounts, feed.OpenAlerts, err)
	}
	stop, err := svc.Process(ctx, "owner", EventRequest{EventID: "stop-1", IncidentKey: "water", Event: "stop"})
	if err != nil || stop.Status != "applied" {
		t.Fatalf("stop: %+v, %v", stop, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status FROM alerts WHERE id=?`, start.AlertID).Scan(&status); err != nil || status != "resolved" {
		t.Fatalf("stop did not resolve canonical alert: %q, %v", status, err)
	}
}

func TestSingleIngestionKeyRotation(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newTestService(t)
	if _, err := db.ExecContext(ctx, `INSERT INTO users(id,name,email,role) VALUES('owner','Owner','owner@example.com','admin')`); err != nil {
		t.Fatal(err)
	}
	first, err := svc.RotateIngestionKey(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if owner, err := svc.ValidateIngestionKey(ctx, first); err != nil || owner != "owner" {
		t.Fatalf("first key: owner=%q err=%v", owner, err)
	}
	second, err := svc.RotateIngestionKey(ctx, "owner")
	if err != nil || second == first {
		t.Fatalf("rotated key: same=%v err=%v", second == first, err)
	}
	if _, err := svc.ValidateIngestionKey(ctx, first); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old key still valid: %v", err)
	}
	if err := svc.RevokeIngestionKey(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateIngestionKey(ctx, second); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked key still valid: %v", err)
	}
}
