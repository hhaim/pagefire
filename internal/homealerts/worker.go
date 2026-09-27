package homealerts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

func (s *Service) Name() string { return "home-alerts" }

func (s *Service) Tick(ctx context.Context) error {
	if err := s.repeatDue(ctx); err != nil {
		return err
	}
	if err := s.weeklyReport(ctx); err != nil {
		return err
	}
	if err := s.dispatchDue(ctx); err != nil {
		return err
	}
	if err := s.processPushoverReceipts(ctx); err != nil {
		return err
	}
	if err := s.pollActions(ctx); err != nil {
		return err
	}
	if time.Since(s.lastClean) >= time.Hour {
		if err := s.cleanup(ctx); err != nil {
			return err
		}
		s.lastClean = time.Now()
	}
	return nil
}

func (s *Service) repeatDue(ctx context.Context) error {
	now := time.Now().UTC().Unix()
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM home_alerts WHERE status='active' AND severity='high' AND acknowledged_at IS NULL AND next_repeat_at <= ? ORDER BY next_repeat_at LIMIT 50`, now)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.repeatOne(ctx, id, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) repeatOne(ctx context.Context, id string, now int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var source, key, severity, summary, details string
	var interval int
	err = tx.QueryRowContext(ctx, `SELECT source,incident_key,severity,summary,details,repeat_interval_seconds FROM home_alerts WHERE id=? AND status='active' AND acknowledged_at IS NULL AND next_repeat_at <= ?`, id, now).Scan(&source, &key, &severity, &summary, &details, &interval)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE home_alerts SET next_repeat_at=? WHERE id=? AND status='active' AND acknowledged_at IS NULL AND next_repeat_at <= ?`, now+int64(interval), id, now)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	eventID := uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO home_events(id,source,alert_id,kind,severity,summary,details,created_at,incident_key) VALUES(?,?,?,'repeat',?,?,?,?,?)`, eventID, source, id, severity, summary, details, now, key)
	if err != nil {
		return err
	}
	if _, err = s.enqueue(ctx, tx, eventID, message("repeat", severity, summary, details, key), now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) dispatchDue(ctx context.Context) error {
	now := time.Now().UTC().Unix()
	rows, err := s.db.QueryContext(ctx, `SELECT id,event_id,plugin_kind,message,attempts FROM home_deliveries WHERE (status='pending' OR status='sending') AND next_attempt_at <= ? ORDER BY created_at LIMIT 50`, now)
	if err != nil {
		return err
	}
	type item struct {
		id, eventID, kind, msg string
		attempts               int
	}
	var items []item
	for rows.Next() {
		var x item
		if err := rows.Scan(&x.id, &x.eventID, &x.kind, &x.msg, &x.attempts); err != nil {
			rows.Close()
			return err
		}
		items = append(items, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, x := range items {
		res, err := s.db.ExecContext(ctx, `UPDATE home_deliveries SET status='sending',next_attempt_at=? WHERE id=? AND (status='pending' OR status='sending') AND next_attempt_at <= ?`, now+60, x.id, now)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		var destination, encrypted string
		var enabled int
		err = s.db.QueryRowContext(ctx, `SELECT destination,secret,enabled FROM home_plugins WHERE kind=?`, x.kind).Scan(&destination, &encrypted, &enabled)
		if err == sql.ErrNoRows {
			s.failDelivery(ctx, x.id, "plugin disabled or removed", x.attempts+1, true)
			continue
		}
		if err != nil {
			s.failDelivery(ctx, x.id, "plugin configuration unavailable", x.attempts+1, false)
			continue
		}
		if enabled == 0 {
			s.failDelivery(ctx, x.id, "plugin disabled or removed", x.attempts+1, true)
			continue
		}
		secret, err := s.decrypt(encrypted)
		if err != nil {
			s.failDelivery(ctx, x.id, "plugin secret could not be read", x.attempts+1, true)
			continue
		}
		provider := s.providers[x.kind]
		if provider == nil {
			s.failDelivery(ctx, x.id, "plugin type unavailable", x.attempts+1, true)
			continue
		}
		var alertID sql.NullString
		var eventKind, severity string
		var interval int
		err = s.db.QueryRowContext(ctx, `SELECT e.alert_id,e.kind,e.severity,COALESCE(a.repeat_interval_seconds,300) FROM home_events e LEFT JOIN home_alerts a ON a.id=e.alert_id WHERE e.id=?`, x.eventID).Scan(&alertID, &eventKind, &severity, &interval)
		if err != nil {
			s.failDelivery(ctx, x.id, "event unavailable", x.attempts+1, true)
			continue
		}
		delivery := Notification{Destination: destination, Secret: secret, Message: x.msg, RepeatIntervalSeconds: interval}
		if severity == "high" && (eventKind == "start" || eventKind == "repeat") {
			delivery.AckAlertID = alertID.String
		}
		if x.kind == "pushover" && severity == "high" && eventKind == "repeat" {
			var acknowledgedAt, canceledAt sql.NullInt64
			var expiresAt int64
			err = s.db.QueryRowContext(ctx, `SELECT acknowledged_at,canceled_at,expires_at FROM home_pushover_receipts WHERE alert_id=?`, alertID.String).Scan(&acknowledgedAt, &canceledAt, &expiresAt)
			if err == nil && !acknowledgedAt.Valid && !canceledAt.Valid && expiresAt > now {
				_, err = s.db.ExecContext(ctx, `UPDATE home_deliveries SET status='cancelled',error='Pushover emergency is already repeating' WHERE id=?`, x.id)
				if err != nil {
					return err
				}
				continue
			}
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			if acknowledgedAt.Valid {
				if _, err := s.db.ExecContext(ctx, `UPDATE home_deliveries SET status='cancelled',error='alert acknowledged' WHERE id=?`, x.id); err != nil {
					return err
				}
				continue
			}
			delivery.Message = "Pushover ACK pending.\n" + delivery.Message
		}
		if pushover, ok := provider.(*pushoverProvider); ok && severity == "high" && (eventKind == "start" || eventKind == "repeat") {
			if eventKind == "start" {
				delivery.Message = "Acknowledge in Pushover to stop repeats.\n" + delivery.Message
			}
			var existing string
			err = s.db.QueryRowContext(ctx, `SELECT receipt FROM home_pushover_receipts WHERE alert_id=?`, alertID.String).Scan(&existing)
			if err == sql.ErrNoRows || eventKind == "repeat" {
				var receipt string
				receipt, err = pushover.SendEmergency(ctx, delivery)
				if err == nil {
					_, err = s.db.ExecContext(ctx, `INSERT INTO home_pushover_receipts(alert_id,receipt,expires_at) VALUES(?,?,?) ON CONFLICT(alert_id) DO UPDATE SET receipt=excluded.receipt,expires_at=excluded.expires_at,acknowledged_at=NULL,canceled_at=NULL,last_checked_at=0`, alertID.String, receipt, time.Now().UTC().Unix()+10800)
				}
			}
		} else if pushover, ok := provider.(*pushoverProvider); ok && severity == "high" {
			// Info and stop events are also emergency priority in Pushover.
			delivery.ExpireSeconds = 300
			_, err = pushover.SendEmergency(ctx, delivery)
		} else {
			err = provider.Send(ctx, delivery)
		}
		if err != nil {
			slog.Warn("home notification failed", "delivery_id", x.id, "plugin", x.kind, "error", err)
			s.failDelivery(ctx, x.id, err.Error(), x.attempts+1, false)
			continue
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE home_deliveries SET status='sent',attempts=?,sent_at=?,error='' WHERE id=?`, x.attempts+1, time.Now().UTC().Unix(), x.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) processPushoverReceipts(ctx context.Context) error {
	provider, ok := s.providers["pushover"].(*pushoverProvider)
	if !ok {
		return nil
	}
	var encrypted string
	err := s.db.QueryRowContext(ctx, `SELECT secret FROM home_plugins WHERE kind='pushover'`).Scan(&encrypted)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	secret, err := s.decrypt(encrypted)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Unix()
	rows, err := s.db.QueryContext(ctx, `SELECT r.alert_id,r.receipt,a.source,a.status,r.expires_at FROM home_pushover_receipts r JOIN home_alerts a ON a.id=r.alert_id WHERE r.acknowledged_at IS NULL AND r.canceled_at IS NULL AND ((a.status='stopped' AND r.last_checked_at<=?) OR (a.status='active' AND r.expires_at>? AND r.last_checked_at<=?)) ORDER BY r.last_checked_at LIMIT 20`, now-5, now, now-30)
	if err != nil {
		return err
	}
	type item struct {
		alertID, receipt, source, status string
		expiresAt                        int64
	}
	var items []item
	for rows.Next() {
		var x item
		if err := rows.Scan(&x.alertID, &x.receipt, &x.source, &x.status, &x.expiresAt); err != nil {
			rows.Close()
			return err
		}
		items = append(items, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, x := range items {
		if _, err := s.db.ExecContext(ctx, `UPDATE home_pushover_receipts SET last_checked_at=? WHERE alert_id=?`, now, x.alertID); err != nil {
			return err
		}
		acknowledged, ackAt, err := provider.ReceiptStatus(ctx, secret, x.receipt)
		if err != nil {
			slog.Warn("pushover emergency status unavailable", "alert_id", x.alertID, "error", err)
			if x.status != "stopped" {
				continue
			}
		}
		if !acknowledged && x.status == "stopped" {
			if x.expiresAt > now {
				if err := provider.CancelReceipt(ctx, secret, x.receipt); err != nil {
					slog.Warn("pushover emergency cancellation failed", "alert_id", x.alertID, "error", err)
					continue
				}
			}
			if _, err := s.db.ExecContext(ctx, `UPDATE home_pushover_receipts SET canceled_at=? WHERE alert_id=?`, now, x.alertID); err != nil {
				return err
			}
			continue
		}
		if !acknowledged {
			continue
		}
		if ackAt == 0 {
			ackAt = now
		}
		if x.status == "active" {
			if err := s.AcknowledgeByID(ctx, x.alertID, "pushover"); err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
		}
		_, err = s.db.ExecContext(ctx, `UPDATE home_pushover_receipts SET acknowledged_at=? WHERE alert_id=? AND acknowledged_at IS NULL`, ackAt, x.alertID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) pollActions(ctx context.Context) error {
	for kind, provider := range s.providers {
		action, ok := provider.(ActionProvider)
		if !ok {
			continue
		}
		if time.Now().Before(s.actionRetryAt[kind]) {
			continue
		}
		var destination, encrypted string
		err := s.db.QueryRowContext(ctx, `SELECT destination,secret FROM home_plugins WHERE kind=? AND enabled=1`, kind).Scan(&destination, &encrypted)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		secret, err := s.decrypt(encrypted)
		if err != nil {
			return err
		}
		if err := action.Poll(ctx, destination, secret, s.AcknowledgeByID); err != nil {
			slog.Warn("home plugin actions unavailable", "plugin", kind, "error", err)
			s.actionRetryAt[kind] = time.Now().Add(time.Minute)
		} else {
			delete(s.actionRetryAt, kind)
		}
	}
	return nil
}

func (s *Service) failDelivery(ctx context.Context, id, reason string, attempts int, terminal bool) {
	status := "pending"
	if terminal || attempts >= 3 {
		status = "failed"
	}
	backoff := int64(30)
	if attempts > 1 {
		backoff = 120
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE home_deliveries SET status=?,attempts=?,next_attempt_at=?,error=? WHERE id=?`, status, attempts, time.Now().UTC().Unix()+backoff, truncate(reason, 300), id); err != nil {
		slog.Error("home delivery status update failed", "delivery_id", id, "error", err)
	}
}

func (s *Service) weeklyReport(ctx context.Context) error {
	return s.weeklyReportAt(ctx, time.Now().UTC())
}

func (s *Service) weeklyReportAt(ctx context.Context, now time.Time) error {
	weekday := (int(now.Weekday()) + 6) % 7 // Monday = 0
	weekStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -weekday)
	if now.Before(weekStart.Add(9 * time.Hour)) {
		return nil
	}
	previous := weekStart.AddDate(0, 0, -7)
	key := "weekly:" + previous.Format("2006-01-02")
	var exists string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM home_events WHERE source='system' AND client_event_id=?`, key).Scan(&exists)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	stats, err := s.Stats(ctx, previous.Unix(), weekStart.Unix())
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("PageFire weekly home alerts (%s–%s)\nStarted: %d (high %d, mid %d, low %d)\nStopped: %d\nInfo: %d\nNotifications: %d sent, %d failed", previous.Format("Jan 2"), weekStart.AddDate(0, 0, -1).Format("Jan 2"), stats.Started, stats.High, stats.Mid, stats.Low, stats.Stopped, stats.Info, stats.Sent, stats.Failed)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	eventID := uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO home_events(id,source,client_event_id,kind,summary,created_at) VALUES(?,'system',?,'weekly_stats',?,?)`, eventID, key, msg, now.Unix())
	if err != nil {
		return err
	}
	if _, err = s.enqueue(ctx, tx, eventID, msg, now.Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) cleanup(ctx context.Context) error {
	cutoff := time.Now().UTC().AddDate(0, -6, 0).Unix()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM home_alerts WHERE status='stopped' AND stopped_at < ?`, cutoff); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM home_events WHERE created_at < ? AND (alert_id IS NULL OR alert_id NOT IN (SELECT id FROM home_alerts WHERE status='active'))`, cutoff)
	return err
}
