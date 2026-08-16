package backend

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

func SafeError(err error) string {
	if err == nil {
		return ""
	}
	message := fmt.Sprint(err)
	if len(message) > 240 {
		return message[:240]
	}
	return message
}

type Worker struct {
	db     *sql.DB
	slots  chan struct{}
	mu     sync.Mutex
	apps   map[string]chan struct{}
	events *Events
	run    func(context.Context, Operation) error
}

func NewWorker(db *sql.DB, run func(context.Context, Operation) error) *Worker {
	return &Worker{db: db, slots: make(chan struct{}, 2), apps: map[string]chan struct{}{}, events: NewEvents(), run: run}
}
func (w *Worker) Enqueue(op Operation) error {
	w.mu.Lock()
	lock := w.apps[op.ApplicationID]
	if lock == nil {
		lock = make(chan struct{}, 1)
		w.apps[op.ApplicationID] = lock
	}
	select {
	case lock <- struct{}{}:
	default:
		w.mu.Unlock()
		return &ConflictError{Message: "同じアプリに未完了のOperationがあります"}
	}
	w.mu.Unlock()
	go w.execute(op, lock)
	return nil
}
func (w *Worker) execute(op Operation, lock chan struct{}) {
	defer func() { <-lock }()
	w.slots <- struct{}{}
	defer func() { <-w.slots }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	_ = SetOperationState(ctx, w.db, op.ID, "RUNNING", "")
	w.events.Publish(op.ID, "running", nil)
	err := error(nil)
	if w.run != nil {
		err = w.run(ctx, op)
	}
	state, msg := "SUCCEEDED", ""
	if err != nil {
		state, msg = "FAILED", SafeError(err)
		_, _ = w.db.ExecContext(ctx, `UPDATE applications SET observed_state='ERROR',latest_error=?,updated_at=datetime('now') WHERE id=?`, msg, op.ApplicationID)
	}
	_ = SetOperationState(ctx, w.db, op.ID, state, msg)
	w.events.Publish(op.ID, stringsLower(state), map[string]string{"message": msg})
}
func stringsLower(s string) string {
	if s == "SUCCEEDED" {
		return "succeeded"
	}
	if s == "FAILED" {
		return "failed"
	}
	return "running"
}

type ConflictError struct{ Message string }

func (e *ConflictError) Error() string { return e.Message }

type event struct {
	ID        string    `json:"eventId"`
	Sequence  int64     `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Data      any       `json:"data"`
}

type eventSubscription struct {
	events <-chan event
	close  func()
}

func (s *eventSubscription) C() <-chan event { return s.events }
func (s *eventSubscription) Close()          { s.close() }

type Events struct {
	mu        sync.Mutex
	seq       int64
	listeners map[string]map[*eventSubscription]chan event
}

func NewEvents() *Events { return &Events{listeners: map[string]map[*eventSubscription]chan event{}} }
func (e *Events) Subscribe(id string) *eventSubscription {
	e.mu.Lock()
	defer e.mu.Unlock()
	c := make(chan event, 16)
	s := &eventSubscription{events: c}
	s.close = func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		if _, ok := e.listeners[id][s]; ok {
			delete(e.listeners[id], s)
			close(c)
		}
	}
	if e.listeners[id] == nil {
		e.listeners[id] = map[*eventSubscription]chan event{}
	}
	e.listeners[id][s] = c
	return s
}
func (e *Events) Publish(id, typ string, data any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.seq++
	value := event{ID: fmt.Sprintf("%s-%d", id, e.seq), Sequence: e.seq, Timestamp: time.Now().UTC(), Type: typ, Data: data}
	for _, c := range e.listeners[id] {
		select {
		case c <- value:
		default:
		}
	}
}
