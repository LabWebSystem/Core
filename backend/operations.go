package backend

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type operationProgressKey struct{}
type operationOutputKey struct{}

func withOperationProgress(ctx context.Context, report func(string, string)) context.Context {
	return context.WithValue(ctx, operationProgressKey{}, report)
}

func reportOperationProgress(ctx context.Context, message string) {
	reportOperationPhase(ctx, "running", message)
}
func reportOperationPhase(ctx context.Context, phase, message string) {
	if report, ok := ctx.Value(operationProgressKey{}).(func(string, string)); ok {
		report(phase, message)
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

type Operation struct{ ID, ApplicationID, RequestID, Kind, State, ErrorMessage, Phase, DisplayMessage, Payload, CreatedAt, UpdatedAt string }

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
	err := db.QueryRowContext(ctx, `SELECT id,application_id,request_id,kind,state,error_message,phase,display_message,request_fingerprint FROM operations WHERE request_id=?`, requestID).Scan(&op.ID, &op.ApplicationID, &op.RequestID, &op.Kind, &op.State, &op.ErrorMessage, &op.Phase, &op.DisplayMessage, &existingFingerprint)
	if err == nil {
		if op.ApplicationID != applicationID || op.Kind != kind || (fingerprint != "" && existingFingerprint != "" && existingFingerprint != fingerprint) {
			return Operation{}, errors.New("requestIdが異なる要求に再利用されています")
		}
		return op, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Operation{}, err
	}
	// 同じ内容の再送は、別の待機ジョブを作らず既存Operationへ収束させる。
	err = db.QueryRowContext(ctx, `SELECT id,application_id,request_id,kind,state,error_message,phase,display_message,payload,created_at,updated_at FROM operations WHERE application_id=? AND kind=? AND request_fingerprint=? AND state IN ('QUEUED','RUNNING') ORDER BY created_at LIMIT 1`, applicationID, kind, fingerprint).Scan(&op.ID, &op.ApplicationID, &op.RequestID, &op.Kind, &op.State, &op.ErrorMessage, &op.Phase, &op.DisplayMessage, &op.Payload, &op.CreatedAt, &op.UpdatedAt)
	if err == nil {
		return op, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Operation{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := uuid.NewString()
	if _, err = db.ExecContext(ctx, `INSERT INTO operations(id,application_id,request_id,kind,state,error_message,phase,display_message,request_fingerprint,payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, applicationID, requestID, kind, "QUEUED", "", "queued", "開始を待機しています", fingerprint, payload, now, now); err != nil {
		return Operation{}, err
	}
	return Operation{ID: id, ApplicationID: applicationID, RequestID: requestID, Kind: kind, State: "QUEUED", Phase: "queued", DisplayMessage: "開始を待機しています", Payload: payload}, nil
}

func SetOperationState(ctx context.Context, db *sql.DB, id, state, message string) error {
	display := "操作が完了しました"
	if state == "FAILED" {
		display = "操作に失敗しました"
	} else if state == "CANCELLED" {
		display = "操作を中止しました"
	}
	_, err := db.ExecContext(ctx, `UPDATE operations SET state=?,error_message=?,phase=?,display_message=?,updated_at=? WHERE id=?`, state, message, strings.ToLower(state), display, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

func SetOperationProgress(ctx context.Context, db *sql.DB, id, phase, message string) error {
	_, err := db.ExecContext(ctx, `UPDATE operations SET phase=?,display_message=?,updated_at=? WHERE id=?`, phase, message, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}
