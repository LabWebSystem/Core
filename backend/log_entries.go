package backend

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	defaultLogLimit = 100
	maxLogLimit     = 500
)

// StoredLogEntryはDocker、Operation、およびBackendのログを共通形式で表す。
// Cursorは永続化せず、occurredAtとidから都度生成するopaque値である。
type StoredLogEntry struct {
	ID            string    `json:"id"`
	Cursor        string    `json:"cursor"`
	OccurredAt    time.Time `json:"occurredAt"`
	Level         string    `json:"level"`
	Component     string    `json:"component"`
	ApplicationID string    `json:"applicationId,omitempty"`
	OperationID   string    `json:"operationId,omitempty"`
	Service       string    `json:"service,omitempty"`
	ContainerName string    `json:"containerName,omitempty"`
	Message       string    `json:"message"`
}

type LogQuery struct {
	ApplicationID string
	View          string
	Service       string
	StartAt       *time.Time
	EndAt         *time.Time
	Cursor        string
	Limit         int
}

type LogPage struct {
	Entries    []StoredLogEntry `json:"entries"`
	NextCursor string           `json:"nextCursor,omitempty"`
	LiveCursor string           `json:"liveCursor,omitempty"`
}

type LogStore struct {
	DB        *sql.DB
	SecretKey []byte
	Now       func() time.Time
	mu        sync.Mutex
	watch     map[chan struct{}]struct{}
}

func NewLogStore(db *sql.DB) *LogStore {
	return &LogStore{DB: db, Now: time.Now, watch: map[chan struct{}]struct{}{}}
}

func (s *LogStore) Append(ctx context.Context, entry StoredLogEntry) (StoredLogEntry, error) {
	if s == nil || s.DB == nil {
		return StoredLogEntry{}, errors.New("ログデータベースを利用できません")
	}
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	if entry.OccurredAt.IsZero() {
		entry.OccurredAt = s.now().UTC()
	} else {
		entry.OccurredAt = entry.OccurredAt.UTC()
	}
	if !validLogLevel(entry.Level) {
		entry.Level = "info"
	}
	if !validLogComponent(entry.Component) {
		return StoredLogEntry{}, errors.New("ログcomponentが不正です")
	}
	entry.Message = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(entry.Message)
	if strings.TrimSpace(entry.Message) == "" {
		return StoredLogEntry{}, errors.New("ログmessageが必要です")
	}
	var err error
	if entry.Message, err = s.redact(ctx, entry.ApplicationID, entry.Message); err != nil {
		return StoredLogEntry{}, err
	}
	if entry.Component != "application" {
		entry.Service = ""
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO log_entries(id,occurred_at,level,component,application_id,operation_id,service,container_name,message) VALUES(?,?,?,?,?,?,?,?,?)`, entry.ID, entry.OccurredAt.Format(time.RFC3339Nano), entry.Level, entry.Component, nullString(entry.ApplicationID), nullString(entry.OperationID), entry.Service, entry.ContainerName, entry.Message); err != nil {
		return StoredLogEntry{}, fmt.Errorf("ログを保存できません: %w", err)
	}
	entry.Cursor = encodeLogCursor(entry.OccurredAt, entry.ID)
	s.notify()
	return entry, nil
}

func (s *LogStore) redact(ctx context.Context, app, message string) (string, error) {
	if app == "" || len(s.SecretKey) == 0 {
		return message, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT value FROM application_variables WHERE application_id=? AND is_secret=1`, app)
	if err != nil {
		return "", fmt.Errorf("ログのsecret設定を取得できません")
	}
	defer rows.Close()
	secrets := []string{}
	for rows.Next() {
		var encrypted []byte
		if err := rows.Scan(&encrypted); err != nil {
			return "", err
		}
		plain, err := Decrypt(s.SecretKey, encrypted)
		if err != nil {
			return "", fmt.Errorf("ログのsecretを復号できません")
		}
		secrets = append(secrets, string(plain))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return redactLog(message, secrets), nil
}

func (s *LogStore) Query(ctx context.Context, q LogQuery) (LogPage, error) {
	if s == nil || s.DB == nil {
		return LogPage{}, errors.New("ログデータベースを利用できません")
	}
	if q.ApplicationID == "" || !validLogView(q.View) || !validLogService(q.Service) || q.Limit < 0 || q.Limit > maxLogLimit {
		return LogPage{}, errors.New("ログ検索条件が不正です")
	}
	if q.Limit == 0 {
		q.Limit = defaultLogLimit
	}
	if q.StartAt != nil && q.EndAt != nil && q.EndAt.Before(*q.StartAt) {
		return LogPage{}, errors.New("ログ検索期間が不正です")
	}
	var cursorAt time.Time
	var cursorID string
	var err error
	if q.Cursor != "" {
		cursorAt, cursorID, err = decodeLogCursor(q.Cursor)
		if err != nil {
			return LogPage{}, errors.New("cursorが不正です")
		}
		var exists int
		if err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM log_entries WHERE id=? AND occurred_at=?`, cursorID, cursorAt.Format(time.RFC3339Nano)).Scan(&exists); err != nil || exists != 1 {
			return LogPage{}, errors.New("cursorが不正です")
		}
	}
	where := []string{"(application_id=? OR operation_id IN (SELECT id FROM operations WHERE application_id=?))"}
	args := []any{q.ApplicationID, q.ApplicationID}
	switch q.View {
	case "task":
		where = append(where, "operation_id IS NOT NULL")
	case "application":
		where = append(where, "component='application'")
	case "related":
		where = append(where, "component!='application'")
	}
	if q.Service != "" {
		where = append(where, "service=?")
		args = append(args, q.Service)
	}
	if q.StartAt != nil {
		where = append(where, "occurred_at>=?")
		args = append(args, q.StartAt.UTC().Format(time.RFC3339Nano))
	}
	if q.EndAt != nil {
		where = append(where, "occurred_at<=?")
		args = append(args, q.EndAt.UTC().Format(time.RFC3339Nano))
	}
	if q.Cursor != "" {
		where = append(where, "(occurred_at>? OR (occurred_at=? AND id>?))")
		args = append(args, cursorAt.Format(time.RFC3339Nano), cursorAt.Format(time.RFC3339Nano), cursorID)
	}
	args = append(args, q.Limit+1)
	rows, err := s.DB.QueryContext(ctx, `SELECT id,occurred_at,level,component,COALESCE(application_id,''),COALESCE(operation_id,''),service,container_name,message FROM log_entries WHERE `+strings.Join(where, " AND ")+` ORDER BY occurred_at,id LIMIT ?`, args...)
	if err != nil {
		return LogPage{}, fmt.Errorf("ログを取得できません: %w", err)
	}
	defer rows.Close()
	entries := make([]StoredLogEntry, 0, q.Limit)
	for rows.Next() {
		var e StoredLogEntry
		var occurred string
		if err := rows.Scan(&e.ID, &occurred, &e.Level, &e.Component, &e.ApplicationID, &e.OperationID, &e.Service, &e.ContainerName, &e.Message); err != nil {
			return LogPage{}, err
		}
		if e.OccurredAt, err = time.Parse(time.RFC3339Nano, occurred); err != nil {
			return LogPage{}, fmt.Errorf("保存済みログ時刻が不正です")
		}
		e.Cursor = encodeLogCursor(e.OccurredAt, e.ID)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return LogPage{}, err
	}
	page := LogPage{Entries: entries}
	if len(entries) > q.Limit {
		page.Entries = entries[:q.Limit]
		page.NextCursor = page.Entries[len(page.Entries)-1].Cursor
	}
	if len(page.Entries) > 0 {
		page.LiveCursor = page.Entries[len(page.Entries)-1].Cursor
	} else {
		page.LiveCursor = q.Cursor
	}
	return page, nil
}

func (s *LogStore) RemoveExpired(ctx context.Context, retention time.Duration, maxBytes int64) error {
	if retention <= 0 || maxBytes <= 0 {
		return errors.New("ログ保持条件が不正です")
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM log_entries WHERE occurred_at<?`, s.now().UTC().Add(-retention).Format(time.RFC3339Nano)); err != nil {
		return err
	}
	for {
		var size int64
		if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(length(message)+length(id)+length(occurred_at)+length(level)+length(component)+length(service)+length(container_name)),0) FROM log_entries`).Scan(&size); err != nil {
			return err
		}
		if size <= maxBytes {
			return nil
		}
		result, err := s.DB.ExecContext(ctx, `DELETE FROM log_entries WHERE id=(SELECT id FROM log_entries ORDER BY occurred_at,id LIMIT 1)`)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return nil
		}
	}
}

func (s *LogStore) Subscribe() (<-chan struct{}, func()) {
	c := make(chan struct{}, 1)
	s.mu.Lock()
	s.watch[c] = struct{}{}
	s.mu.Unlock()
	return c, func() {
		s.mu.Lock()
		if _, ok := s.watch[c]; ok {
			delete(s.watch, c)
			close(c)
		}
		s.mu.Unlock()
	}
}
func (s *LogStore) notify() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.watch {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}
func (s *LogStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func validLogLevel(value string) bool {
	switch value {
	case "debug", "info", "warn", "error":
		return true
	}
	return false
}
func validLogComponent(value string) bool {
	switch value {
	case "backend", "caddy", "coredns", "dashboard", "application":
		return true
	}
	return false
}
func validLogView(value string) bool {
	return value == "task" || value == "application" || value == "related"
}

var composeServicePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)

func validLogService(value string) bool {
	return value == "" || composeServicePattern.MatchString(value)
}
func encodeLogCursor(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "\x00" + id))
}
func decodeLogCursor(value string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.Split(string(raw), "\x00")
	if len(parts) != 2 || parts[1] == "" {
		return time.Time{}, "", errors.New("invalid")
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	return at, parts[1], err
}

// ChunkRedactorはsecretがchunk境界をまたいでも平文を返さない。
type ChunkRedactor struct {
	secrets []string
	pending string
}

func NewChunkRedactor(secrets []string) *ChunkRedactor {
	r := &ChunkRedactor{}
	for _, secret := range secrets {
		if secret != "" {
			r.secrets = append(r.secrets, secret)
		}
	}
	return r
}
func (r *ChunkRedactor) Feed(chunk string) string {
	r.pending += chunk
	n := len(r.pending) - r.pendingSecretPrefix()
	out := redactLog(r.pending[:n], r.secrets)
	r.pending = r.pending[n:]
	return out
}

func (r *ChunkRedactor) pendingSecretPrefix() int {
	longest := 0
	for _, secret := range r.secrets {
		for n := 1; n < len(secret) && n <= len(r.pending); n++ {
			if strings.HasSuffix(r.pending, secret[:n]) && n > longest {
				longest = n
			}
		}
	}
	return longest
}
func (r *ChunkRedactor) Flush() string {
	out := redactLog(r.pending, r.secrets)
	r.pending = ""
	return out
}
func redactLog(value string, secrets []string) string {
	for _, secret := range secrets {
		value = strings.ReplaceAll(value, secret, "[REDACTED]")
	}
	return value
}
