package homealerts

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"time"
)

type FeedFilter struct {
	Window       string
	Type         string
	Class        string
	Source       string
	Client       string
	Open         *bool
	MessageRegex string
	Limit        int
	Offset       int
}

type FeedEvent struct {
	ID            string `json:"id"`
	Origin        string `json:"origin"`
	Type          string `json:"type"`
	Class         string `json:"class"`
	Source        string `json:"source"`
	Client        string `json:"client"`
	Open          bool   `json:"open"`
	Status        string `json:"status"`
	Summary       string `json:"summary"`
	Details       string `json:"details"`
	AlertID       string `json:"alert_id,omitempty"`
	ClientEventID string `json:"event_id,omitempty"`
	CreatedAt     int64  `json:"created_at"`
}

type FeedBucket struct {
	Time    int64 `json:"time"`
	Info    int   `json:"info"`
	Warning int   `json:"warning"`
	Error   int   `json:"error"`
}

type FeedFacets struct {
	Types   []string `json:"types"`
	Sources []string `json:"sources"`
	Clients []string `json:"clients"`
}

type FeedResponse struct {
	Events     []FeedEvent    `json:"events"`
	Buckets    []FeedBucket   `json:"buckets"`
	TypeCounts map[string]int `json:"type_counts"`
	Facets     FeedFacets     `json:"facets"`
	Total      int            `json:"total"`
	OpenAlerts int            `json:"open_alerts"`
	From       int64          `json:"from"`
	To         int64          `json:"to"`
}

func feedWindow(window string) (int64, int64, error) {
	switch window {
	case "1h":
		return 3600, 60, nil
	case "1d":
		return 86400, 1800, nil
	case "1w":
		return 7 * 86400, 10800, nil
	case "1m":
		return 30 * 86400, 43200, nil
	default:
		return 0, 0, fmt.Errorf("%w: window must be 1h, 1d, 1w, or 1m", ErrInvalid)
	}
}

func feedClass(severity string) string {
	switch severity {
	case "high":
		return "error"
	case "mid":
		return "warning"
	default:
		return "info"
	}
}

func sortedFacet(values map[string]struct{}) []string {
	items := make([]string, 0, len(values))
	for value := range values {
		if value != "" {
			items = append(items, value)
		}
	}
	sort.Strings(items)
	return items
}

func (s *Service) Feed(ctx context.Context, userID string, filter FeedFilter) (FeedResponse, error) {
	windowSeconds, bucketSeconds, err := feedWindow(filter.Window)
	if err != nil {
		return FeedResponse{}, err
	}
	var messagePattern *regexp.Regexp
	if filter.MessageRegex != "" {
		if len(filter.MessageRegex) > 256 {
			return FeedResponse{}, fmt.Errorf("%w: message regex is too long", ErrInvalid)
		}
		messagePattern, err = regexp.Compile("(?i)" + filter.MessageRegex)
		if err != nil {
			return FeedResponse{}, fmt.Errorf("%w: invalid message regex: %v", ErrInvalid, err)
		}
	}

	to := time.Now().UTC().Unix() + 1
	from := to - windowSeconds
	all := []FeedEvent{}

	homeRows, err := s.db.QueryContext(ctx, `SELECT e.id,e.client_event_id,e.alert_id,e.kind,e.severity,e.summary,e.details,e.created_at,e.incident_key,COALESCE(h.status,''),h.acknowledged_at IS NOT NULL FROM home_events e LEFT JOIN home_alerts h ON h.id=e.alert_id WHERE e.source=? AND e.created_at>=? AND e.created_at<?`, userID, from, to)
	if err != nil {
		return FeedResponse{}, err
	}
	for homeRows.Next() {
		var event FeedEvent
		var clientEventID, alertID sql.NullString
		var severity string
		var acknowledged bool
		if err := homeRows.Scan(&event.ID, &clientEventID, &alertID, &event.Type, &severity, &event.Summary, &event.Details, &event.CreatedAt, &event.Client, &event.Status, &acknowledged); err != nil {
			homeRows.Close()
			return FeedResponse{}, err
		}
		event.Origin, event.Source, event.Class = "home", "home", feedClass(severity)
		event.Open = event.Status == "active"
		if event.Open && acknowledged {
			event.Status = "acknowledged"
		}
		event.AlertID, event.ClientEventID = alertID.String, clientEventID.String
		all = append(all, event)
	}
	err = homeRows.Err()
	homeRows.Close()
	if err != nil {
		return FeedResponse{}, err
	}

	types, sources, clients := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	counts := map[string]int{}
	matched := []FeedEvent{}
	openAlertIDs := map[string]struct{}{}
	buckets := map[int64]*FeedBucket{}
	for _, event := range all {
		types[event.Type] = struct{}{}
		sources[event.Source] = struct{}{}
		clients[event.Client] = struct{}{}
		if filter.Class != "" && event.Class != filter.Class || filter.Source != "" && event.Source != filter.Source || filter.Client != "" && event.Client != filter.Client || filter.Open != nil && event.Open != *filter.Open {
			continue
		}
		if messagePattern != nil && !messagePattern.MatchString(event.Summary+"\n"+event.Details) {
			continue
		}
		counts[event.Type]++
		if filter.Type != "" && event.Type != filter.Type {
			continue
		}
		matched = append(matched, event)
		if event.Open && event.AlertID != "" {
			openAlertIDs[event.AlertID] = struct{}{}
		}
		bucketTime := event.CreatedAt / bucketSeconds * bucketSeconds
		bucket := buckets[bucketTime]
		if bucket == nil {
			bucket = &FeedBucket{Time: bucketTime}
			buckets[bucketTime] = bucket
		}
		switch event.Class {
		case "error":
			bucket.Error++
		case "warning":
			bucket.Warning++
		default:
			bucket.Info++
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].CreatedAt == matched[j].CreatedAt {
			return matched[i].ID > matched[j].ID
		}
		return matched[i].CreatedAt > matched[j].CreatedAt
	})

	response := FeedResponse{Events: []FeedEvent{}, Buckets: []FeedBucket{}, TypeCounts: counts, Total: len(matched), OpenAlerts: len(openAlertIDs), From: from, To: to}
	response.Facets = FeedFacets{Types: sortedFacet(types), Sources: sortedFacet(sources), Clients: sortedFacet(clients)}
	for bucketTime := from / bucketSeconds * bucketSeconds; bucketTime < to; bucketTime += bucketSeconds {
		if bucket := buckets[bucketTime]; bucket != nil {
			response.Buckets = append(response.Buckets, *bucket)
		} else {
			response.Buckets = append(response.Buckets, FeedBucket{Time: bucketTime})
		}
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if filter.Limit <= 0 || filter.Limit > 250 {
		filter.Limit = 100
	}
	if filter.Offset < len(matched) {
		end := filter.Offset + filter.Limit
		if end > len(matched) {
			end = len(matched)
		}
		response.Events = matched[filter.Offset:end]
	}
	return response, nil
}
