package backend

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

type operationProgressKey struct{}
type operationOutputKey struct{}

func withOperationProgress(ctx context.Context, report func(string)) context.Context {
	return context.WithValue(ctx, operationProgressKey{}, report)
}

func reportOperationProgress(ctx context.Context, message string) {
	if report, ok := ctx.Value(operationProgressKey{}).(func(string)); ok {
		report(message)
	}
}
func withOperationOutput(ctx context.Context, report func(task, message, level string)) context.Context {
	return context.WithValue(ctx, operationOutputKey{}, report)
}
func reportOperationOutput(ctx context.Context, task, message, level string) {
	if report, ok := ctx.Value(operationOutputKey{}).(func(string, string, string)); ok && message != "" {
		report(task, message, level)
	}
}

type Operation struct{ ID, ApplicationID, RequestID, Kind, State, ErrorMessage, Payload string }

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func CreateOperation(ctx context.Context, db *sql.DB, applicationID, requestID, kind string) (Operation, error) {
	return createOperation(ctx, db, applicationID, requestID, kind, "", "")
}

func CreateOperationWithFingerprint(ctx context.Context, db *sql.DB, applicationID, requestID, kind, fingerprint string) (Operation, error) {
	return createOperation(ctx, db, applicationID, requestID, kind, fingerprint, "")
}

func CreateOperationWithPayload(ctx context.Context, db *sql.DB, applicationID, requestID, kind, fingerprint, payload string) (Operation, error) {
	return createOperation(ctx, db, applicationID, requestID, kind, fingerprint, payload)
}

func createOperation(ctx context.Context, db *sql.DB, applicationID, requestID, kind, fingerprint, payload string) (Operation, error) {
	return createOperationWithExecutor(ctx, db, applicationID, requestID, kind, fingerprint, payload)
}

func createOperationWithExecutor(ctx context.Context, db sqlExecutor, applicationID, requestID, kind, fingerprint, payload string) (Operation, error) {
	if err := ValidateRequestID(requestID); err != nil {
		return Operation{}, errors.New("requestIdはUUID v4で指定してください")
	}
	var op Operation
	var existingFingerprint string
	err := db.QueryRowContext(ctx, `SELECT id,application_id,request_id,kind,state,error_message,request_fingerprint FROM operations WHERE request_id=?`, requestID).Scan(&op.ID, &op.ApplicationID, &op.RequestID, &op.Kind, &op.State, &op.ErrorMessage, &existingFingerprint)
	if err == nil {
		if op.ApplicationID != applicationID || op.Kind != kind || (fingerprint != "" && existingFingerprint != "" && existingFingerprint != fingerprint) {
			return Operation{}, errors.New("requestIdが異なる要求に再利用されています")
		}
		return op, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Operation{}, err
	}
	var active int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM operations WHERE application_id=? AND state IN ('QUEUED','RUNNING')`, applicationID).Scan(&active); err != nil {
		return Operation{}, err
	}
	if active > 0 {
		return Operation{}, &ConflictError{Message: "同じアプリに未完了のOperationがあります"}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := uuid.NewString()
	if _, err = db.ExecContext(ctx, `INSERT INTO operations(id,application_id,request_id,kind,state,error_message,request_fingerprint,payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, applicationID, requestID, kind, "QUEUED", "", fingerprint, payload, now, now); err != nil {
		return Operation{}, err
	}
	return Operation{ID: id, ApplicationID: applicationID, RequestID: requestID, Kind: kind, State: "QUEUED", Payload: payload}, nil
}

func SetOperationState(ctx context.Context, db *sql.DB, id, state, message string) error {
	_, err := db.ExecContext(ctx, `UPDATE operations SET state=?,error_message=?,updated_at=datetime('now') WHERE id=?`, state, message, id)
	return err
}
