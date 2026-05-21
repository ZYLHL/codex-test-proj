package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

type SyncStore interface {
	Push(context.Context, PushOp) (PushAck, error)
	Pull(context.Context, string, int) ([]Change, string, error)
	Full(context.Context, int) ([]Change, string, error)
}

type SyncService struct{ store SyncStore }

func NewSyncService(s SyncStore) *SyncService { return &SyncService{store: s} }

type PushRequest struct {
	DeviceID string   `json:"deviceId"`
	Ops      []PushOp `json:"ops"`
}

type PushOp struct {
	IdempotencyKey string  `json:"idempotencyKey"`
	NoteID         string  `json:"noteId"`
	OpType         string  `json:"opType"`
	Payload        Payload `json:"payload"`
}

type Payload struct {
	CipherText      string `json:"cipherText"`
	Nonce           string `json:"nonce"`
	AAD             string `json:"aad"`
	ClientUpdatedAt string `json:"clientUpdatedAt"`
	Deleted         bool   `json:"deleted"`
}

type PushAck struct {
	NoteID          string `json:"noteId"`
	ServerUpdatedAt string `json:"serverUpdatedAt"`
	Deduplicated    bool   `json:"deduplicated"`
}

type PushResponse struct {
	Accepted []PushAck  `json:"accepted"`
	Rejected []Rejected `json:"rejected"`
}

type Rejected struct {
	NoteID string `json:"noteId"`
	Reason string `json:"reason"`
}

type Change struct {
	NoteID          string `json:"noteId"`
	CipherText      string `json:"cipherText"`
	Nonce           string `json:"nonce"`
	AAD             string `json:"aad"`
	Deleted         bool   `json:"deleted"`
	ServerUpdatedAt string `json:"serverUpdatedAt"`
}

func (s *SyncService) Push(ctx context.Context, req PushRequest) (PushResponse, error) {
	resp := PushResponse{Accepted: []PushAck{}, Rejected: []Rejected{}}
	for _, op := range req.Ops {
		if err := validateOp(req.DeviceID, op); err != nil {
			resp.Rejected = append(resp.Rejected, Rejected{NoteID: op.NoteID, Reason: err.Error()})
			continue
		}
		ack, err := s.store.Push(ctx, op)
		if err != nil {
			return resp, err
		}
		resp.Accepted = append(resp.Accepted, ack)
	}
	return resp, nil
}

func (s *SyncService) Pull(ctx context.Context, since string, limit int) ([]Change, string, error) {
	return s.store.Pull(ctx, since, limit)
}
func (s *SyncService) Full(ctx context.Context, limit int) ([]Change, string, error) {
	return s.store.Full(ctx, limit)
}

func validateOp(deviceID string, op PushOp) error {
	if strings.TrimSpace(deviceID) == "" || strings.TrimSpace(op.NoteID) == "" || strings.TrimSpace(op.OpType) == "" {
		return fmt.Errorf("missing required fields")
	}
	if op.OpType != "upsert" && op.OpType != "delete" {
		return fmt.Errorf("unsupported opType")
	}
	if op.IdempotencyKey == "" {
		op.IdempotencyKey = genIdempotencyKey(deviceID, op.NoteID, op.OpType)
	}
	return nil
}

func genIdempotencyKey(deviceID, noteID, opType string) string {
	h := sha256.Sum256([]byte(deviceID + ":" + noteID + ":" + opType + ":" + time.Now().UTC().Format(time.RFC3339Nano)))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func DecodeCursor(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return 0, err
	}
	var seq int64
	if _, err := fmt.Sscanf(string(b), "v1:%d", &seq); err != nil {
		return 0, err
	}
	return seq, nil
}

func EncodeCursor(seq int64) string {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("v1:%d", seq)))
}

func IsNotFound(err error) bool { return err == sql.ErrNoRows }
