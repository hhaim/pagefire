package pagefire_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pagefire/pagefire/internal/api"
	"github.com/pagefire/pagefire/internal/auth"
	"github.com/pagefire/pagefire/internal/homealerts"
	"github.com/pagefire/pagefire/internal/store"
	"github.com/pagefire/pagefire/internal/store/sqlite"
)

// newE2ERouter creates an in-memory store, router, and admin session for e2e tests.
func newE2ERouter(t *testing.T) (http.Handler, *sqlite.SQLiteStore, string) {
	t.Helper()
	ctx := context.Background()

	s, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	authSvc := auth.NewService(s.Users(), s.DB())
	homeSvc, err := homealerts.New(s.DB(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	router := api.NewRouter(s, authSvc, homeSvc)

	// Create an admin user and log in through the session API.
	hash, err := auth.HashPassword("testpass123")
	if err != nil {
		t.Fatal(err)
	}
	adminUser := &store.User{
		Name:         "Test Admin",
		Email:        "admin@e2e.dev",
		Role:         store.RoleAdmin,
		Timezone:     "UTC",
		PasswordHash: hash,
		IsActive:     true,
	}
	if err := s.Users().Create(ctx, adminUser); err != nil {
		t.Fatal(err)
	}
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"admin@e2e.dev","password":"testpass123"}`))
	login.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	router.ServeHTTP(loginRR, login)
	if loginRR.Code != http.StatusOK || len(loginRR.Result().Cookies()) == 0 {
		t.Fatalf("login: status=%d body=%s", loginRR.Code, loginRR.Body.String())
	}
	cookie := loginRR.Result().Cookies()[0]
	return router, s, cookie.Name + "=" + cookie.Value
}

func TestHomeEventsAPI(t *testing.T) {
	router, _, token := newE2ERouter(t)
	do := func(method, path, body string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Cookie", token)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		var response map[string]any
		if rr.Body.Len() > 0 && strings.Contains(rr.Header().Get("Content-Type"), "application/json") {
			if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
		}
		return rr.Code, response
	}
	code, result := do("POST", "/api/v1/events", `{"event_id":"event-1","event":"start","incident_key":"door","severity":"high","summary":"Door open"}`)
	if code != 201 || result["status"] != "applied" {
		t.Fatalf("start: %d %+v", code, result)
	}
	alertID, ok := result["alert_id"].(string)
	if !ok || alertID == "" {
		t.Fatalf("no alert ID: %+v", result)
	}
	code, result = do("POST", "/api/v1/events", `{"event_id":"event-1","event":"start","incident_key":"door","severity":"high","summary":"Door open"}`)
	if code != 200 || result["status"] != "already_seen" {
		t.Fatalf("retry: %d %+v", code, result)
	}
	code, result = do("GET", "/api/v1/events/event-1", "")
	if code != 200 || result["kind"] != "start" {
		t.Fatalf("detail: %d %+v", code, result)
	}
	code, result = do("GET", "/api/v1/home-alerts/"+alertID, "")
	if code != 200 || result["incident_key"] != "door" {
		t.Fatalf("active home alert: %d %+v", code, result)
	}
	code, result = do("POST", "/api/v1/events", `{"event_id":"event-2","event":"stop","incident_key":"door"}`)
	if code != 201 || result["status"] != "applied" {
		t.Fatalf("stop: %d %+v", code, result)
	}
	code, _ = do("GET", "/api/v1/home-alerts/"+alertID, "")
	if code != 404 {
		t.Fatalf("stopped alert should not be active: %d", code)
	}
	code, result = do("GET", "/api/v1/alert-events?window=1h", "")
	if code != 200 || result["total"] != float64(2) {
		t.Fatalf("combined event feed: %d %+v", code, result)
	}
	code, result = do("GET", "/api/v1/home-stats", "")
	if code != 200 || result["started"] != float64(1) || result["stopped"] != float64(1) {
		t.Fatalf("stats: %d %+v", code, result)
	}
	for _, path := range []string{"/api/v1/services", "/api/v1/escalation-policies", "/api/v1/integrations/old/alerts", "/api/v1/alerts", "/api/v1/auth/tokens"} {
		code, _ = do("GET", path, "")
		if code != http.StatusNotFound {
			t.Fatalf("legacy route %s: status=%d, want 404", path, code)
		}
	}
	code, result = do("PUT", "/api/v1/home-plugins/telegram", `{"destination":"1234","secret":"123:abc","enabled":false}`)
	if code != 200 || result["configured"] != true || result["secret"] != nil {
		t.Fatalf("plugin configuration: %d %+v", code, result)
	}
}
