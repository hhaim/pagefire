package homealerts

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pagefire/pagefire/internal/store/sqlite"
)

type fakeProvider struct {
	mu       sync.Mutex
	messages []string
}

func (*fakeProvider) Kind() string                              { return "telegram" }
func (*fakeProvider) Name() string                              { return "Fake Telegram" }
func (*fakeProvider) DestinationLabel() string                  { return "Chat ID" }
func (*fakeProvider) SecretLabel() string                       { return "Bot token" }
func (*fakeProvider) Validate(destination, secret string) error { return nil }
func (f *fakeProvider) Send(_ context.Context, delivery Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, delivery.Message)
	return nil
}
func (f *fakeProvider) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.messages) }
func (f *fakeProvider) sentMessages() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.messages...)
}

func newTestService(t *testing.T) (*Service, *sql.DB, *fakeProvider) {
	t.Helper()
	dir := t.TempDir()
	store, err := sqlite.New(filepath.Join(dir, "pagefire.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc, err := New(store.DB(), dir)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeProvider{}
	svc.Register(fake)
	if _, err := svc.PutPlugin(context.Background(), "telegram", PluginInput{Destination: "chat", Secret: "secret", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	return svc, store.DB(), fake
}

func TestHomeAlertLifecycleAndDelivery(t *testing.T) {
	ctx := context.Background()
	svc, db, provider := newTestService(t)
	start := EventRequest{EventID: "start-1", IncidentKey: "water", Event: "start", Severity: "high", Summary: "Water detected", Details: "Water level above threshold", RepeatIntervalSeconds: 300}
	result, err := svc.Process(ctx, "source-1", start)
	if err != nil || result.Status != "applied" || result.Deliveries != 1 {
		t.Fatalf("start: %+v, %v", result, err)
	}
	if result.AlertID == "" {
		t.Fatal("start returned no alert ID")
	}
	var startedAt, nextRepeatAt int64
	var repeatInterval int
	if err := db.QueryRowContext(ctx, `SELECT started_at,next_repeat_at,repeat_interval_seconds FROM home_alerts WHERE id=?`, result.AlertID).Scan(&startedAt, &nextRepeatAt, &repeatInterval); err != nil {
		t.Fatal(err)
	}
	if repeatInterval != 300 || nextRepeatAt-startedAt != 300 {
		t.Fatalf("high alert repeat schedule: interval=%d, next-start delta=%d; want 300 seconds", repeatInterval, nextRepeatAt-startedAt)
	}

	retry, err := svc.Process(ctx, "source-1", start)
	if err != nil || retry.Status != "already_seen" || retry.Deliveries != 0 {
		t.Fatalf("retry: %+v, %v", retry, err)
	}
	start.EventID = "start-2"
	duplicate, err := svc.Process(ctx, "source-1", start)
	if err != nil || duplicate.Status != "duplicate_active" || duplicate.Deliveries != 0 {
		t.Fatalf("duplicate active: %+v, %v", duplicate, err)
	}

	_, err = db.ExecContext(ctx, `UPDATE home_alerts SET next_repeat_at=? WHERE id=?`, time.Now().Unix()-1, result.AlertID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.repeatDue(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.dispatchDue(ctx); err != nil {
		t.Fatal(err)
	}
	if provider.count() != 2 {
		t.Fatalf("wanted initial and repeat notifications, got %d", provider.count())
	}
	messages := provider.sentMessages()
	if !strings.Contains(messages[0], start.Details) || !strings.Contains(messages[1], start.Details) {
		t.Fatalf("start/repeat plugin messages omitted details: %q", messages)
	}
	_, err = db.ExecContext(ctx, `UPDATE home_alerts SET next_repeat_at=? WHERE id=?`, time.Now().Unix()-1, result.AlertID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.repeatDue(ctx); err != nil {
		t.Fatal(err)
	}

	if err := svc.Acknowledge(ctx, "source-1", result.AlertID, "user-1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Acknowledge(ctx, "source-1", result.AlertID, "user-1"); err != nil {
		t.Fatalf("ack retry: %v", err)
	}
	if err := svc.repeatDue(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.dispatchDue(ctx); err != nil {
		t.Fatal(err)
	}
	if provider.count() != 2 {
		t.Fatal("acknowledged alert repeated")
	}
	var cancelled int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM home_deliveries d JOIN home_events e ON e.id=d.event_id WHERE e.alert_id=? AND e.kind='repeat' AND d.status='cancelled'`, result.AlertID).Scan(&cancelled); err != nil {
		t.Fatal(err)
	}
	if cancelled != 1 {
		t.Fatalf("wanted 1 cancelled repeat, got %d", cancelled)
	}

	stop, err := svc.Process(ctx, "source-1", EventRequest{EventID: "stop-1", IncidentKey: "water", Event: "stop"})
	if err != nil || stop.Status != "applied" {
		t.Fatalf("stop: %+v, %v", stop, err)
	}
	if err := svc.dispatchDue(ctx); err != nil {
		t.Fatal(err)
	}
	if provider.count() != 3 {
		t.Fatal("stop notification was not sent")
	}
	stopMessage := provider.sentMessages()[2]
	if !strings.Contains(stopMessage, "DOWN — all clear: Water detected") || !strings.Contains(stopMessage, start.Details) {
		t.Fatalf("stop plugin message did not confirm recovery with details: %q", stopMessage)
	}

	info, err := svc.Process(ctx, "source-1", EventRequest{EventID: "info-1", IncidentKey: "door", Event: "info", Summary: "Door opened"})
	if err != nil || info.Status != "applied" || info.AlertID != "" {
		t.Fatalf("info: %+v, %v", info, err)
	}
	if err := svc.dispatchDue(ctx); err != nil {
		t.Fatal(err)
	}
	if provider.count() != 4 {
		t.Fatal("info notification was not sent")
	}

	detail, err := svc.GetEvent(ctx, "source-1", "info-1")
	if err != nil || len(detail.Deliveries) != 1 || detail.Deliveries[0].Status != "sent" {
		t.Fatalf("info delivery: %+v, %v", detail, err)
	}
	stats, err := svc.Stats(ctx, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())
	if err != nil || stats.Started != 1 || stats.Stopped != 1 || stats.Info != 1 {
		t.Fatalf("stats: %+v, %v", stats, err)
	}
	var storedSecret string
	if err := db.QueryRowContext(ctx, `SELECT secret FROM home_plugins WHERE kind='telegram'`).Scan(&storedSecret); err != nil {
		t.Fatal(err)
	}
	if storedSecret == "secret" {
		t.Fatal("plugin secret stored as plaintext")
	}
}

func TestWeeklyReportAndRetention(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newTestService(t)
	weekStart := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	previous := weekStart.AddDate(0, 0, -7)
	_, err := db.ExecContext(ctx, `INSERT INTO home_events(id,source,client_event_id,kind,severity,summary,created_at) VALUES('weekly-source','source-1','old-event','start','high','old alert',?)`, previous.Add(time.Hour).Unix())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.weeklyReportAt(ctx, weekStart.Add(10*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := svc.weeklyReportAt(ctx, weekStart.Add(11*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM home_events WHERE source='system' AND client_event_id='weekly:2026-09-14'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("wanted one weekly report, got %d", count)
	}
	var msg string
	if err := db.QueryRowContext(ctx, `SELECT message FROM home_deliveries WHERE event_id=(SELECT id FROM home_events WHERE client_event_id='weekly:2026-09-14')`).Scan(&msg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "Started: 1") {
		t.Fatalf("wrong weekly report: %q", msg)
	}

	old := time.Now().UTC().AddDate(0, -7, 0).Unix()
	_, err = db.ExecContext(ctx, `INSERT INTO home_alerts(id,source,incident_key,severity,status,summary,started_at,stopped_at) VALUES('old-stopped','source-1','old-key','low','stopped','old',?,?)`, old, old)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO home_alerts(id,source,incident_key,severity,status,summary,started_at) VALUES('old-active','source-1','active-key','low','active','still active',?)`, old)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO home_events(id,source,client_event_id,alert_id,kind,summary,created_at) VALUES('old-active-event','source-1','active-event','old-active','start','still active',?)`, old)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO home_events(id,source,client_event_id,kind,summary,created_at) VALUES('old-info','source-1','old-info','info','expired',?)`, old)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM home_alerts WHERE id IN ('old-stopped','old-active')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("wanted only active old alert retained, got %d", count)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM home_events WHERE id IN ('old-info','old-active-event')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("wanted only active alert history retained, got %d", count)
	}
}
