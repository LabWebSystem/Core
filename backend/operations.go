package backend

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Operation struct{ ID, ApplicationID, RequestID, Kind, State, ErrorMessage string }

func CreateOperation(ctx context.Context, db *sql.DB, applicationID, requestID, kind string) (Operation, error) {
	if _, err := uuid.Parse(requestID); err != nil {
		return Operation{}, errors.New("requestIdはUUIDで指定してください")
	}
	var op Operation
	err := db.QueryRowContext(ctx, `SELECT id,application_id,request_id,kind,state,error_message FROM operations WHERE request_id=?`, requestID).Scan(&op.ID, &op.ApplicationID, &op.RequestID, &op.Kind, &op.State, &op.ErrorMessage)
	if err == nil {
		if op.ApplicationID != applicationID || op.Kind != kind {
			return Operation{}, errors.New("requestIdが異なる要求に再利用されています")
		}
		return op, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Operation{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := uuid.NewString()
	if _, err = db.ExecContext(ctx, `INSERT INTO operations(id,application_id,request_id,kind,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, id, applicationID, requestID, kind, "QUEUED", now, now); err != nil {
		return Operation{}, err
	}
	return Operation{ID: id, ApplicationID: applicationID, RequestID: requestID, Kind: kind, State: "QUEUED"}, nil
}

func SetOperationState(ctx context.Context, db *sql.DB, id, state, message string) error {
	_, err := db.ExecContext(ctx, `UPDATE operations SET state=?,error_message=?,updated_at=datetime('now') WHERE id=?`, state, message, id)
	return err
}
