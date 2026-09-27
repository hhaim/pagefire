package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pagefire/pagefire/internal/auth"
	"github.com/pagefire/pagefire/internal/homealerts"
	"github.com/pagefire/pagefire/internal/store"
	"github.com/pagefire/pagefire/internal/store/sqlite"
)

func TestEventIngestionKeyCanOnlySubmitEvents(t *testing.T) {
	ctx := context.Background()
	s, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	user := &store.User{Name: "Owner", Email: "owner@example.com", Role: store.RoleAdmin, Timezone: "UTC", IsActive: true}
	if err := s.Users().Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	home, err := homealerts.New(s.DB(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key, err := home.RotateIngestionKey(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(s, auth.NewService(s.Users(), s.DB()), home)
	doRequest := func(method, path string, body any, token string) *httptest.ResponseRecorder {
		var payload []byte
		if body != nil {
			payload, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}
	request := map[string]any{"event_id": "external-1", "event": "start", "incident_key": "water", "severity": "high", "summary": "Water leak", "details": "Boiler room sensor"}
	if rr := doRequest(http.MethodPost, "/api/v1/events", request, key); rr.Code != http.StatusCreated {
		t.Fatalf("ingest status=%d body=%s", rr.Code, rr.Body.String())
	}
	for _, path := range []string{"/api/v1/home-alerts/active", "/api/v1/events", "/api/v1/event-ingestion-key"} {
		if rr := doRequest(http.MethodGet, path, nil, key); rr.Code != http.StatusUnauthorized {
			t.Fatalf("ingestion key read %s: status=%d", path, rr.Code)
		}
	}
	rotated, err := home.RotateIngestionKey(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	request["event_id"] = "external-2"
	if rr := doRequest(http.MethodPost, "/api/v1/events", request, key); rr.Code != http.StatusUnauthorized {
		t.Fatalf("old key after rotation: status=%d", rr.Code)
	}
	if rr := doRequest(http.MethodPost, "/api/v1/events", request, rotated); rr.Code != http.StatusOK {
		t.Fatalf("new key after rotation: status=%d body=%s", rr.Code, rr.Body.String())
	}
}
