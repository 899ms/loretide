package topicplanning

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

var errReadFailed = errors.New("read failed")

type failingDatabase struct{}

func (failingDatabase) Begin(context.Context) (pgx.Tx, error) { return nil, errReadFailed }
func (failingDatabase) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errReadFailed
}
func (failingDatabase) QueryRow(context.Context, string, ...any) pgx.Row {
	return failingRow{}
}

type failingRow struct{}

func (failingRow) Scan(...any) error { return errReadFailed }

type recordingDiagnostics struct{ technical []diagnostics.Event }

func (*recordingDiagnostics) AuditTx(context.Context, pgx.Tx, diagnostics.Scope, diagnostics.Event) error {
	return nil
}
func (r *recordingDiagnostics) Technical(_ context.Context, event diagnostics.Event) {
	r.technical = append(r.technical, event)
}

func TestReadFailureProducesSanitizedTechnicalEvent(t *testing.T) {
	recorder := &recordingDiagnostics{}
	store := &Store{DB: failingDatabase{}, Diagnostics: recorder, Build: "test"}
	_, err := store.Get(t.Context(), "workspace-a", "actor-a", "topic-a")
	if !errors.Is(err, ErrStorage) {
		t.Fatalf("get = %v, want ErrStorage", err)
	}
	if len(recorder.technical) != 1 {
		t.Fatalf("technical event count = %d, want 1", len(recorder.technical))
	}
	event := recorder.technical[0]
	if event.Component != "topic-planning" || event.Severity != "error" ||
		event.Action != "execute" || event.Outcome != "failed" ||
		event.Code != "DATABASE_UNAVAILABLE" || event.Step != "get" {
		t.Fatalf("unexpected technical event: %#v", event)
	}
}
