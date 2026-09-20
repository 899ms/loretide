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

// countingDatabase answers every call the way failingDatabase does and counts
// the transactions that were opened.
type countingDatabase struct {
	failingDatabase
	begins int
}

func (d *countingDatabase) Begin(ctx context.Context) (pgx.Tx, error) {
	d.begins++
	return d.failingDatabase.Begin(ctx)
}

// A store with no workspace delete fence has no way to honour the delete/write
// protocol, so it must refuse before it opens a transaction rather than write
// rows that a committed workspace deletion can no longer sweep. The fence
// itself is exercised against the real schema by internal/handler's
// TestContentTopicWritesAreFencedByWorkspaceDeletion.
func TestWritesFailClosedWithoutTheWorkspaceFence(t *testing.T) {
	database := &countingDatabase{}
	store := &Store{DB: database, Diagnostics: &recordingDiagnostics{}, Build: "test"}
	writes := map[string]func() error{
		"create": func() error {
			_, err := store.Create(t.Context(), "actor-a", TopicCard{WorkspaceID: "workspace-a"})
			return err
		},
		"act": func() error {
			_, err := store.Act(t.Context(), "workspace-a", "actor-a", "topic-a",
				ActionRequest{Action: ActionStart})
			return err
		},
		"append-brief": func() error {
			_, err := store.AppendBrief(t.Context(), "workspace-a", "actor-a", "topic-a", BriefRevision{})
			return err
		},
		"set-account": func() error {
			_, err := store.SetAccount(t.Context(), "workspace-a", "actor-a", "topic-a", nil)
			return err
		},
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			if err := write(); !errors.Is(err, ErrStorage) {
				t.Fatalf("%s without a guard = %v, want ErrStorage", name, err)
			}
		})
	}
	if database.begins != 0 {
		t.Fatalf("opened %d transactions without a guard, want 0", database.begins)
	}
}
