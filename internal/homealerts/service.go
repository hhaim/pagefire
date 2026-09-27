package homealerts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrInvalid = errors.New("invalid home alert event")
var ErrNotFound = errors.New("home alert not found")

type EventRequest struct {
	EventID               string `json:"event_id"`
	IncidentKey           string `json:"incident_key"`
	Event                 string `json:"event"`
	Severity              string `json:"severity"`
	Summary               string `json:"summary"`
	Details               string `json:"details"`
	RepeatIntervalSeconds int    `json:"repeat_interval_seconds"`
}

type EventResult struct {
	EventID    string `json:"event_id"`
	AlertID    string `json:"alert_id,omitempty"`
	Status     string `json:"status"`
	Deliveries int    `json:"deliveries"`
}

type Delivery struct {
	ID         string `json:"id"`
	PluginKind string `json:"plugin_kind"`
	Status     string `json:"status"`
	Attempts   int    `json:"attempts"`
	Error      string `json:"error,omitempty"`
	SentAt     *int64 `json:"sent_at,omitempty"`
}

type EventDetail struct {
	ID            string     `json:"id"`
	ClientEventID string     `json:"event_id,omitempty"`
	AlertID       string     `json:"alert_id,omitempty"`
	Kind          string     `json:"kind"`
	Severity      string     `json:"severity"`
	Summary       string     `json:"summary"`
	Details       string     `json:"details"`
	CreatedAt     int64      `json:"created_at"`
	Deliveries    []Delivery `json:"deliveries,omitempty"`
}

type Stats struct {
	From    int64 `json:"from"`
	To      int64 `json:"to"`
	Started int   `json:"started"`
	Stopped int   `json:"stopped"`
	Info    int   `json:"info"`
	High    int   `json:"high"`
	Mid     int   `json:"mid"`
	Low     int   `json:"low"`
	Sent    int   `json:"sent"`
	Failed  int   `json:"failed"`
}

type Service struct {
	db            *sql.DB
	providers     map[string]Provider
	key           []byte
	lastClean     time.Time
	actionRetryAt map[string]time.Time
}

func New(db *sql.DB, dataDir string) (*Service, error) {
	key, err := loadKey(dataDir)
	if err != nil {
		return nil, err
	}
	s := &Service{db: db, key: key, providers: map[string]Provider{}, actionRetryAt: map[string]time.Time{}}
	s.Register(NewTelegram())
	s.Register(NewPushover())
	return s, nil
}

func (s *Service) Register(p Provider) { s.providers[p.Kind()] = p }

func validate(req *EventRequest) error {
	if strings.TrimSpace(req.EventID) == "" || len(req.EventID) > 128 {
		return fmt.Errorf("%w: event_id is required (max 128 characters)", ErrInvalid)
	}
	if req.Event != "start" && req.Event != "stop" && req.Event != "info" {
		return fmt.Errorf("%w: event must be start, stop, or info", ErrInvalid)
	}
	if len(req.IncidentKey) > 256 || ((req.Event == "start" || req.Event == "stop") && req.IncidentKey == "") {
		return fmt.Errorf("%w: incident_key is required for start/stop (max 256 characters)", ErrInvalid)
	}
	if req.Severity == "" && req.Event == "info" {
		req.Severity = "low"
	}
	if req.Event != "stop" && req.Severity != "low" && req.Severity != "mid" && req.Severity != "high" {
		return fmt.Errorf("%w: severity must be low, mid, or high", ErrInvalid)
	}
	if req.Event != "stop" && (strings.TrimSpace(req.Summary) == "" || len(req.Summary) > 500) {
		return fmt.Errorf("%w: summary is required (max 500 characters)", ErrInvalid)
	}
	if len(req.Details) > 10000 {
		return fmt.Errorf("%w: details exceed 10000 characters", ErrInvalid)
	}
	if len(req.Summary) > 500 {
		return fmt.Errorf("%w: summary exceeds 500 characters", ErrInvalid)
	}
	if req.Event == "start" && req.Severity == "high" {
		if req.RepeatIntervalSeconds == 0 {
			req.RepeatIntervalSeconds = 300
		}
		if req.RepeatIntervalSeconds < 300 {
			return fmt.Errorf("%w: repeat interval must be at least 300 seconds", ErrInvalid)
		}
	} else if req.RepeatIntervalSeconds != 0 {
		return fmt.Errorf("%w: repeat interval applies only to high start events", ErrInvalid)
	}
	return nil
}

func (s *Service) Process(ctx context.Context, source string, req EventRequest) (EventResult, error) {
	if err := validate(&req); err != nil {
		return EventResult{}, err
	}
	now := time.Now().UTC().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EventResult{}, err
	}
	defer tx.Rollback()

	var existingID string
	var existingAlert sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id, alert_id FROM home_events WHERE source = ? AND client_event_id = ?`, source, req.EventID).Scan(&existingID, &existingAlert)
	if err == nil {
		return EventResult{EventID: req.EventID, AlertID: existingAlert.String, Status: "already_seen"}, nil
	}
	if err != sql.ErrNoRows {
		return EventResult{}, err
	}

	result := EventResult{EventID: req.EventID}
	eventKind := req.Event
	switch req.Event {
	case "start":
		var activeID string
		err = tx.QueryRowContext(ctx, `SELECT id FROM home_alerts WHERE source = ? AND incident_key = ? AND status = 'active'`, source, req.IncidentKey).Scan(&activeID)
		if err == nil {
			result.AlertID, result.Status = activeID, "duplicate_active"
			eventKind = "ignored_start"
		} else if err != sql.ErrNoRows {
			return EventResult{}, err
		} else {
			result.AlertID, result.Status = uuid.NewString(), "applied"
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO escalation_policies(id,name,description,repeat) VALUES('home-events-policy','Home Events','Managed by the home event worker',0)`); err != nil {
				return EventResult{}, err
			}
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO services(id,name,description,escalation_policy_id) VALUES('home-events','Home Events','Alerts created by the event API and playground','home-events-policy')`); err != nil {
				return EventResult{}, err
			}
			var next any
			if req.Severity == "high" {
				next = now + int64(req.RepeatIntervalSeconds)
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO home_alerts(id,source,incident_key,severity,summary,details,repeat_interval_seconds,next_repeat_at,started_at) VALUES(?,?,?,?,?,?,?,?,?)`, result.AlertID, source, req.IncidentKey, req.Severity, req.Summary, req.Details, req.RepeatIntervalSeconds, next, now)
			if err != nil {
				return EventResult{}, err
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO alerts(id,service_id,status,summary,details,source,dedup_key,group_key,escalation_policy_snapshot,next_escalation_at,created_at,severity) VALUES(?,'home-events','triggered',?,?,'home',?,?,'{}',NULL,datetime(?,'unixepoch'),?)`, result.AlertID, req.Summary, req.Details, source+":"+req.IncidentKey, req.IncidentKey, now, req.Severity)
			if err != nil {
				return EventResult{}, err
			}
		}
	case "stop":
		var severity, summary, details string
		err = tx.QueryRowContext(ctx, `SELECT id,severity,summary,details FROM home_alerts WHERE source = ? AND incident_key = ? AND status = 'active'`, source, req.IncidentKey).Scan(&result.AlertID, &severity, &summary, &details)
		if err == sql.ErrNoRows {
			result.Status, eventKind = "not_active", "ignored_stop"
			result.AlertID = ""
		} else if err != nil {
			return EventResult{}, err
		} else {
			result.Status = "applied"
			req.Severity = severity
			if req.Summary == "" {
				req.Summary = summary
			}
			if req.Details == "" {
				req.Details = details
			}
			_, err = tx.ExecContext(ctx, `UPDATE home_alerts SET status = 'stopped', stopped_at = ?, next_repeat_at = NULL WHERE id = ?`, now, result.AlertID)
			if err != nil {
				return EventResult{}, err
			}
			_, err = tx.ExecContext(ctx, `UPDATE alerts SET status='resolved',resolved_at=datetime(?,'unixepoch'),next_escalation_at=NULL WHERE id=?`, now, result.AlertID)
			if err != nil {
				return EventResult{}, err
			}
			if err := cancelPendingRepeats(ctx, tx, result.AlertID, "alert stopped"); err != nil {
				return EventResult{}, err
			}
		}
	case "info":
		result.Status = "applied"
	}

	eventID := uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO home_events(id,source,client_event_id,alert_id,kind,severity,summary,details,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, eventID, source, req.EventID, nullable(result.AlertID), eventKind, req.Severity, req.Summary, req.Details, now)
	if err != nil {
		return EventResult{}, err
	}
	if result.Status == "applied" {
		result.Deliveries, err = s.enqueue(ctx, tx, eventID, message(req.Event, req.Severity, req.Summary, req.Details, req.IncidentKey), now)
		if err != nil {
			return EventResult{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return EventResult{}, err
	}
	return result, nil
}

func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func cancelPendingRepeats(ctx context.Context, tx *sql.Tx, alertID, reason string) error {
	_, err := tx.ExecContext(ctx, `UPDATE home_deliveries SET status='cancelled',error=? WHERE status='pending' AND event_id IN (SELECT id FROM home_events WHERE alert_id=? AND kind='repeat')`, reason, alertID)
	return err
}

func message(kind, severity, summary, details, key string) string {
	text := strings.ToUpper(kind) + " [" + strings.ToUpper(severity) + "] " + summary
	if kind == "stop" {
		text = "✅ DOWN — all clear: " + summary
	}
	if key != "" {
		text += "\nKey: " + key
	}
	if details != "" {
		text += "\n" + details
	}
	return text
}

func (s *Service) enqueue(ctx context.Context, tx *sql.Tx, eventID, msg string, now int64) (int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT kind FROM home_plugins WHERE enabled = 1`)
	if err != nil {
		return 0, err
	}
	var kinds []string
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			rows.Close()
			return 0, err
		}
		kinds = append(kinds, kind)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, kind := range kinds {
		_, err = tx.ExecContext(ctx, `INSERT INTO home_deliveries(id,event_id,plugin_kind,message,next_attempt_at,created_at) VALUES(?,?,?,?,?,?)`, uuid.NewString(), eventID, kind, msg, now, now)
		if err != nil {
			return 0, err
		}
	}
	return len(kinds), nil
}

func (s *Service) GetEvent(ctx context.Context, source, clientID string) (EventDetail, error) {
	var d EventDetail
	var alert sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id,client_event_id,alert_id,kind,severity,summary,details,created_at FROM home_events WHERE source=? AND client_event_id=?`, source, clientID).Scan(&d.ID, &d.ClientEventID, &alert, &d.Kind, &d.Severity, &d.Summary, &d.Details, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}
	d.AlertID = alert.String
	d.Deliveries = []Delivery{}
	rows, err := s.db.QueryContext(ctx, `SELECT id,plugin_kind,status,attempts,error,sent_at FROM home_deliveries WHERE event_id=? ORDER BY created_at`, d.ID)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var x Delivery
		var sent sql.NullInt64
		if err := rows.Scan(&x.ID, &x.PluginKind, &x.Status, &x.Attempts, &x.Error, &sent); err != nil {
			return d, err
		}
		if sent.Valid {
			x.SentAt = &sent.Int64
		}
		d.Deliveries = append(d.Deliveries, x)
	}
	return d, rows.Err()
}

func (s *Service) ListEvents(ctx context.Context, source string) ([]EventDetail, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.id,e.client_event_id,e.alert_id,e.kind,e.severity,COALESCE(NULLIF(e.summary,''),a.summary,''),e.details,e.created_at FROM home_events e LEFT JOIN home_alerts a ON a.id=e.alert_id WHERE e.source=? ORDER BY e.created_at DESC,e.id DESC LIMIT 20`, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []EventDetail{}
	for rows.Next() {
		var event EventDetail
		var clientID, alertID sql.NullString
		if err := rows.Scan(&event.ID, &clientID, &alertID, &event.Kind, &event.Severity, &event.Summary, &event.Details, &event.CreatedAt); err != nil {
			return nil, err
		}
		event.ClientEventID = clientID.String
		event.AlertID = alertID.String
		items = append(items, event)
	}
	return items, rows.Err()
}

func (s *Service) Acknowledge(ctx context.Context, source, alertID, actor string) error {
	now := time.Now().UTC().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE home_alerts SET acknowledged_by=?,acknowledged_at=?,next_repeat_at=NULL WHERE id=? AND source=? AND status='active' AND severity='high' AND acknowledged_at IS NULL`, actor, now, alertID, source)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var acknowledged sql.NullInt64
		err := tx.QueryRowContext(ctx, `SELECT acknowledged_at FROM home_alerts WHERE id=? AND source=? AND status='active' AND severity='high'`, alertID, source).Scan(&acknowledged)
		if err == nil && acknowledged.Valid {
			return nil
		}
		return ErrNotFound
	}
	if err := cancelPendingRepeats(ctx, tx, alertID, "alert acknowledged"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE alerts SET status='acknowledged',acknowledged_at=datetime(?,'unixepoch'),next_escalation_at=NULL WHERE id=?`, now, alertID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO home_events(id,source,alert_id,kind,created_at) VALUES(?,?,?,'acknowledge',?)`, uuid.NewString(), source, alertID, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) AcknowledgeByID(ctx context.Context, alertID, actor string) error {
	var source string
	err := s.db.QueryRowContext(ctx, `SELECT source FROM home_alerts WHERE id=?`, alertID).Scan(&source)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return s.Acknowledge(ctx, source, alertID, actor)
}

func (s *Service) Stats(ctx context.Context, from, to int64) (Stats, error) {
	stats := Stats{From: from, To: to}
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FILTER (WHERE kind='start'),COUNT(*) FILTER (WHERE kind='stop'),COUNT(*) FILTER (WHERE kind='info'),COUNT(*) FILTER (WHERE kind='start' AND severity='high'),COUNT(*) FILTER (WHERE kind='start' AND severity='mid'),COUNT(*) FILTER (WHERE kind='start' AND severity='low') FROM home_events WHERE created_at >= ? AND created_at < ?`, from, to).Scan(&stats.Started, &stats.Stopped, &stats.Info, &stats.High, &stats.Mid, &stats.Low)
	if err != nil {
		return stats, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FILTER (WHERE d.status='sent'),COUNT(*) FILTER (WHERE d.status='failed') FROM home_deliveries d JOIN home_events e ON e.id=d.event_id WHERE e.created_at >= ? AND e.created_at < ?`, from, to).Scan(&stats.Sent, &stats.Failed)
	return stats, err
}
