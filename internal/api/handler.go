package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"sticky-notes/internal/service"
)

type Syncer interface {
	Push(context.Context, service.PushRequest) (service.PushResponse, error)
	Pull(context.Context, string, int) ([]service.Change, string, error)
	Full(context.Context, int) ([]service.Change, string, error)
}

type Pinger interface{ Ping(ctx context.Context) error }

type Handler struct {
	sync  Syncer
	ping  Pinger
	token string
}

func NewHandler(s Syncer, p Pinger, token string) *Handler { return &Handler{sync: s, ping: p, token: token} }

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/api/v1/sync/push", h.auth(h.push))
	mux.HandleFunc("/api/v1/sync/pull", h.auth(h.pull))
	mux.HandleFunc("/api/v1/sync/full", h.auth(h.full))
	return mux
}

func (h *Handler) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if a == "" || a != h.token { writeErr(w, http.StatusUnauthorized, "unauthorized"); return }
		next(w, r)
	}
}

func (h *Handler) push(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { writeErr(w, 400, "method not allowed"); return }
	var req service.PushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeErr(w, 400, "invalid json"); return }
	resp, err := h.sync.Push(r.Context(), req)
	if err != nil { writeErr(w, 500, err.Error()); return }
	writeJSON(w, 200, resp)
}

func (h *Handler) pull(w http.ResponseWriter, r *http.Request) {
	changes, next, err := h.sync.Pull(r.Context(), r.URL.Query().Get("since"), 100)
	if err != nil { writeErr(w, 400, "invalid cursor"); return }
	writeJSON(w, 200, map[string]any{"changes": changes, "nextCursor": next})
}
func (h *Handler) full(w http.ResponseWriter, r *http.Request) {
	changes, next, err := h.sync.Full(r.Context(), 100)
	if err != nil { writeErr(w, 500, err.Error()); return }
	writeJSON(w, 200, map[string]any{"changes": changes, "nextCursor": next})
}
func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	if err := h.ping.Ping(r.Context()); err != nil { writeErr(w, 500, "db unavailable"); return }
	writeJSON(w, 200, map[string]any{"status": "ok"})
}

func writeErr(w http.ResponseWriter, code int, msg string) { writeJSON(w, code, map[string]string{"error": msg}) }
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
