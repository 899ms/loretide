// Package idempotency stores replay-safe results for the historical import flow.
package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

const MaxKeyBytes = 255

var (
	ErrInvalid  = errors.New("invalid idempotency request")
	ErrConflict = errors.New("idempotency key conflicts with different input")
	ErrStorage  = errors.New("idempotency storage unavailable")
)

// Request is one caller-visible deduplication slot. The caller supplies a
// resource scope because the same client key is valid for distinct import steps.
type Request struct {
	Operation   string
	Resource    string
	Key         string
	Fingerprint string
}

func NewRequest(operation, resource, key string, input any) (Request, error) {
	if operation == "" || key == "" || len(key) > MaxKeyBytes {
		return Request{}, ErrInvalid
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return Request{}, ErrInvalid
	}
	hash := sha256.Sum256(payload)
	return Request{Operation: operation, Resource: resource, Key: key,
		Fingerprint: hex.EncodeToString(hash[:])}, nil
}

// Claim reserves a request inside the caller's existing write transaction. A
// contender only sees a completed row after the first transaction commits.
func Claim(ctx context.Context, tx pgx.Tx, workspaceID string, request Request) (json.RawMessage, bool, error) {
	var fingerprint string
	var completed bool
	var response json.RawMessage
	err := tx.QueryRow(ctx, `INSERT INTO content_import_idempotency
		(workspace_id, operation, resource_scope, idempotency_key, request_fingerprint)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (workspace_id, operation, resource_scope, idempotency_key) DO NOTHING
		RETURNING request_fingerprint, completed, response_body`,
		workspaceID, request.Operation, request.Resource, request.Key, request.Fingerprint).
		Scan(&fingerprint, &completed, &response)
	if err == nil {
		return nil, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, ErrStorage
	}
	err = tx.QueryRow(ctx, `SELECT request_fingerprint, completed, response_body
		FROM content_import_idempotency
		WHERE workspace_id=$1 AND operation=$2 AND resource_scope=$3 AND idempotency_key=$4
		FOR UPDATE`, workspaceID, request.Operation, request.Resource, request.Key).
		Scan(&fingerprint, &completed, &response)
	if err != nil {
		return nil, false, ErrStorage
	}
	if fingerprint != request.Fingerprint {
		return nil, false, ErrConflict
	}
	if !completed {
		return nil, false, ErrStorage
	}
	return response, true, nil
}

// Complete saves the exact original response before the enclosing write commits.
func Complete(ctx context.Context, tx pgx.Tx, workspaceID string, request Request, response any) error {
	payload, err := json.Marshal(response)
	if err != nil {
		return ErrStorage
	}
	tag, err := tx.Exec(ctx, `UPDATE content_import_idempotency
		SET completed=true, response_body=$6::jsonb
		WHERE workspace_id=$1 AND operation=$2 AND resource_scope=$3
		  AND idempotency_key=$4 AND request_fingerprint=$5`,
		workspaceID, request.Operation, request.Resource, request.Key, request.Fingerprint, payload)
	if err != nil || tag.RowsAffected() != 1 {
		return ErrStorage
	}
	return nil
}
