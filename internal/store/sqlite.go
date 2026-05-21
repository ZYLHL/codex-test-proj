package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"sticky-notes/internal/service"
)

type SQLiteStore struct{ db *sql.DB }

func NewSQLiteStore(db *sql.DB) *SQLiteStore { return &SQLiteStore{db: db} }

func (s *SQLiteStore) Migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS encrypted_notes (
  note_id TEXT NOT NULL UNIQUE,
  cipher_text TEXT NOT NULL,
  nonce TEXT NOT NULL,
  aad TEXT,
  deleted INTEGER NOT NULL DEFAULT 0,
  server_updated_at TEXT NOT NULL,
  server_seq INTEGER PRIMARY KEY AUTOINCREMENT
);
CREATE TABLE IF NOT EXISTS idempotency_log (
  key TEXT PRIMARY KEY,
  note_id TEXT NOT NULL,
  response_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);
`)
	return err
}

func (s *SQLiteStore) Push(ctx context.Context, op service.PushOp) (service.PushAck, error) {
	if op.IdempotencyKey != "" {
		var raw string
		err := s.db.QueryRowContext(ctx, `SELECT response_json FROM idempotency_log WHERE key=?`, op.IdempotencyKey).Scan(&raw)
		if err == nil {
			var ack service.PushAck
			_ = json.Unmarshal([]byte(raw), &ack)
			ack.Deduplicated = true
			return ack, nil
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO encrypted_notes(note_id,cipher_text,nonce,aad,deleted,server_updated_at)
VALUES(?,?,?,?,?,?)
ON CONFLICT(note_id) DO UPDATE SET
cipher_text=excluded.cipher_text, nonce=excluded.nonce, aad=excluded.aad, deleted=excluded.deleted, server_updated_at=excluded.server_updated_at
`, op.NoteID, op.Payload.CipherText, op.Payload.Nonce, op.Payload.AAD, boolToInt(op.Payload.Deleted), now)
	if err != nil { return service.PushAck{}, err }
	ack := service.PushAck{NoteID: op.NoteID, ServerUpdatedAt: now, Deduplicated: false}
	if op.IdempotencyKey != "" {
		raw,_ := json.Marshal(ack)
		_, _ = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO idempotency_log(key,note_id,response_json,created_at) VALUES(?,?,?,?)`, op.IdempotencyKey, op.NoteID, string(raw), now)
	}
	return ack, nil
}

func (s *SQLiteStore) Pull(ctx context.Context, since string, limit int) ([]service.Change, string, error) {
	seq, err := service.DecodeCursor(since); if err != nil { return nil, "", err }
	if limit <= 0 { limit = 100 }
	rows, err := s.db.QueryContext(ctx, `SELECT note_id,cipher_text,nonce,aad,deleted,server_updated_at,server_seq FROM encrypted_notes WHERE server_seq>? ORDER BY server_seq ASC LIMIT ?`, seq, limit)
	if err != nil { return nil, "", err }
	defer rows.Close()
	changes := []service.Change{}
	last := seq
	for rows.Next() {
		var c service.Change; var deleted int; var sseq int64
		if err := rows.Scan(&c.NoteID,&c.CipherText,&c.Nonce,&c.AAD,&deleted,&c.ServerUpdatedAt,&sseq); err != nil { return nil, "", err }
		c.Deleted = deleted == 1
		changes = append(changes,c)
		last = sseq
	}
	return changes, service.EncodeCursor(last), rows.Err()
}

func (s *SQLiteStore) Full(ctx context.Context, limit int) ([]service.Change, string, error) { return s.Pull(ctx, "", limit) }

func (s *SQLiteStore) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func boolToInt(b bool) int { if b { return 1 }; return 0 }
