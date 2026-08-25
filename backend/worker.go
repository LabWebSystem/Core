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
	db        *sql.DB
	slots     chan struct{}
	mu        sync.Mutex
	queues    map[string][]Operation
	running   map[string]bool
	scheduled map[string]bool
	events    *Events
	logs      *LogStore
	run       func(context.Context, Operation) error
}

func NewWorker(db *sql.DB, run func(context.Context, Operation) error) *Worker {
	return &Worker{db: db, slots: make(chan struct{}, 2), queues: map[string][]Operation{}, running: map[string]bool{}, scheduled: map[string]bool{}, events: NewEvents(), run: run}
}
func (w *Worker) Enqueue(op Operation) error {
	if op.State != "" && op.State != "QUEUED" {
		return nil
	}
	w.mu.Lock()
	if w.scheduled[op.ID] {
		w.mu.Unlock()
		return nil
	}
	w.scheduled[op.ID] = true
	w.queues[op.ApplicationID] = append(w.queues[op.ApplicationID], op)
	if !w.running[op.ApplicationID] {
		w.running[op.ApplicationID] = true
		go w.executeApplication(op.ApplicationID)
	}
	w.mu.Unlock()
	return nil
}
func (w *Worker) executeApplication(applicationID string) {
	for {
		w.mu.Lock()
		queue := w.queues[applicationID]
		if len(queue) == 0 {
			delete(w.queues, applicationID)
			delete(w.running, applicationID)
			w.mu.Unlock()
			return
		}
		op := queue[0]
		w.queues[applicationID] = queue[1:]
		w.mu.Unlock()
		w.execute(op)
	}
}
func (w *Worker) execute(op Operation) {
	defer func() {
		w.mu.Lock()
		delete(w.scheduled, op.ID)
		w.mu.Unlock()
	}()
	w.slots <- struct{}{}
	defer func() { <-w.slots }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	ctx = withOperationProgress(ctx, func(phase, message string) {
		_ = SetOperationProgress(ctx, w.db, op.ID, phase, message)
		w.events.Publish(op.ID, "running", map[string]string{"message": message, "phase": phase})
		w.appendLog(ctx, op, "operation", message, "info")
	})
	ctx = withOperationOutput(ctx, func(task, message, level string) { w.appendLog(ctx, op, task, message, level) })
	_ = SetOperationState(ctx, w.db, op.ID, "RUNNING", "")
	reportOperationPhase(ctx, "starting", "操作を開始しています")
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
	display := "操作が完了しました"
	if state == "FAILED" {
		display = "操作に失敗しました"
	} else if state == "CANCELLED" {
		display = "操作を中止しました"
	}
	w.events.Publish(op.ID, stringsLower(state), map[string]string{"message": display, "phase": stringsLower(state)})
	level := "info"
	if state == "FAILED" {
		level = "error"
	}
	w.appendLog(ctx, op, "operation", display, level)
}

func (w *Worker) appendLog(ctx context.Context, op Operation, task, message, level string) {
	if w.logs == nil {
		return
	}
	_, _ = w.logs.Append(ctx, StoredLogEntry{Level: level, Component: "backend", ApplicationID: op.ApplicationID, OperationID: op.ID, Message: "[" + task + "] " + message})
}
func stringsLower(s string) string {
	if s == "SUCCEEDED" {
		return "succeeded"
	}
	if s == "FAILED" {
		return "failed"
	}
	if s == "CANCELLED" {
		return "cancelled"
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
