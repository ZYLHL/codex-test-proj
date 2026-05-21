package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sticky-notes/internal/service"
)

type mockSync struct{}
func (m mockSync) Push(context.Context, service.PushRequest) (service.PushResponse, error) { return service.PushResponse{}, nil }
func (m mockSync) Pull(context.Context, string, int) ([]service.Change, string, error) { return []service.Change{}, "", nil }
func (m mockSync) Full(context.Context, int) ([]service.Change, string, error) { return []service.Change{}, "", nil }

type mockPing struct{ err error }
func (m mockPing) Ping(context.Context) error { return m.err }

func TestAuthFail(t *testing.T) {
	h := NewHandler(mockSync{}, mockPing{}, "token")
	r := httptest.NewRequest(http.MethodPost, "/api/v1/sync/push", strings.NewReader(`{}`))
	w := httptest.NewRecorder(); h.Routes().ServeHTTP(w,r)
	if w.Code != 401 { t.Fatalf("want 401 got %d", w.Code) }
}

func TestPushBadJSON(t *testing.T) {
	h := NewHandler(mockSync{}, mockPing{}, "token")
	r := httptest.NewRequest(http.MethodPost, "/api/v1/sync/push", strings.NewReader(`{`))
	r.Header.Set("Authorization","Bearer token")
	w := httptest.NewRecorder(); h.Routes().ServeHTTP(w,r)
	if w.Code != 400 { t.Fatalf("want 400 got %d", w.Code) }
}
func TestHealthzOK(t *testing.T) {
	h := NewHandler(mockSync{}, mockPing{}, "token")
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder(); h.Routes().ServeHTTP(w,r)
	if w.Code != 200 { t.Fatalf("want 200 got %d", w.Code) }
}
