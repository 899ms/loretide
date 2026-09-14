package diagnostics

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresOutbox is the durable half of the transactional dispatch contract.
// Register writes the record through the caller's own transaction, so the
// record exists if and only if that transaction commits; Settle dispatches the
// batch once it has. A process that dies between the two loses nothing: the row
// is on disk, and Drainer picks it up. That is the whole difference from
// MemoryOutbox, and TestPostgresOutboxSurvivesTheProcess is its assertion.
//
// What this does NOT promise: exactly-once across a crash between a successful
// delivery and the row being marked delivered. Nothing without a distributed
// transaction can promise that, so the receiver has to be idempotent. Every
// other case — concurrent drainers, an expired lease, a process that exits
// before dispatching — is handled here.
type PostgresOutbox struct {
	// Deliver sends one record. Nil means dispatch is a no-op that still marks
	// the record delivered, which is what an installation with no receiver
	// wired should do — the same meaning MemoryOutbox gives a nil Deliver.
	Deliver func(context.Context, DispatchItem) error

	pool   *pgxpool.Pool
	log    *LogBuffer
	limits OutboxLimits

	mu sync.Mutex
	// staged maps a transaction handle to the rows registered against it. The
	// handle is a grouping key only: by the time Settle runs, the transaction
	// has committed and cannot execute anything.
	staged map[any][]int64
}

// OutboxLimits bounds what one record may cost. Both have package defaults; the
// server passes real values in, so a test can shrink them without waiting.
type OutboxLimits struct {
	// MaxPayload is the largest payload accepted. Beyond it the record is
	// dropped and counted rather than written.
	MaxPayload int
	// MaxAttempts is how many deliveries a record gets before it is dead
	// lettered. Zero or less means the default.
	MaxAttempts int
}

// Package defaults. The real values come from the server wiring, which reads
// the environment; this package never does.
const (
	DefaultOutboxMaxPayload  = 1 << 20
	DefaultOutboxMaxAttempts = 8
	DefaultDrainInterval     = 30 * time.Second
	DefaultDrainLease        = time.Minute
	DefaultDrainBatch        = 50
	outboxBackoffBase        = 5 * time.Second
	outboxBackoffCap         = 10 * time.Minute
)

// ErrNoTransaction is what Register returns when the handle it was given is not
// a live database transaction. Failing here is deliberate: staging the record
// or accepting it silently would report it as durable when nothing was written.
var ErrNoTransaction = errors.New("diagnostics: outbox requires the caller's database transaction")

func (l OutboxLimits) maxPayload() int {
	if l.MaxPayload > 0 {
		return l.MaxPayload
	}
	return DefaultOutboxMaxPayload
}

func (l OutboxLimits) maxAttempts() int {
	if l.MaxAttempts > 0 {
		return l.MaxAttempts
	}
	return DefaultOutboxMaxAttempts
}

// NewPostgresOutbox returns an outbox that writes to pool. log receives the
// failure and drop counts, in the same two counters MemoryOutbox already uses -
// Overview reports them as sink_errors and dropped, and this feature adds no
// third one.
func NewPostgresOutbox(pool *pgxpool.Pool, log *LogBuffer, limits OutboxLimits) *PostgresOutbox {
	return &PostgresOutbox{pool: pool, log: log, limits: limits, staged: map[any][]int64{}}
}

func (o *PostgresOutbox) count(drop bool) {
	if o.log == nil {
		return
	}
	if drop {
		o.log.Dropped.Add(1)
		return
	}
	o.log.Errors.Add(1)
}

// Register writes item through tx. It must be the caller's own transaction:
// writing on another connection would break "committed implies registered" in
// exactly the crash window this feature exists to close.
func (o *PostgresOutbox) Register(ctx context.Context, tx any, item DispatchItem) error {
	if o.pool == nil {
		return ErrUnavailable
	}
	// An oversized record is dropped and counted, and Register still returns
	// nil: losing one diagnostic record must not fail the caller's business
	// transaction, which has nothing to do with diagnostics.
	if len(item.Payload) > o.limits.maxPayload() {
		o.count(true)
		return nil
	}
	live, ok := tx.(pgx.Tx)
	if !ok {
		return ErrNoTransaction
	}
	var sequence int64
	err := live.QueryRow(ctx,
		`INSERT INTO content_dispatch_outbox(item_id,kind,payload,idempotency_key,workspace_id)
		 VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING RETURNING sequence`,
		item.ID, item.Kind, item.Payload, item.IdempotencyKey, item.Workspace).Scan(&sequence)
	if errors.Is(err, pgx.ErrNoRows) {
		// A non-empty idempotency key already present. The partial unique index
		// is what decides this, so an empty key never deduplicates - the same
		// answer MemoryOutbox gives.
		return nil
	}
	if err != nil {
		return ErrUnavailable
	}
	o.mu.Lock()
	o.staged[tx] = append(o.staged[tx], sequence)
	o.mu.Unlock()
	return nil
}

// Settle dispatches what tx registered, once tx has committed.
//
// tx is a grouping key here and nothing more. The transaction is already over
// by the time this runs, so the rows are re-read through the pool.
func (o *PostgresOutbox) Settle(ctx context.Context, tx any, committed bool) int {
	o.mu.Lock()
	pending := o.staged[tx]
	delete(o.staged, tx)
	o.mu.Unlock()

	// Staging is cleared either way, so settling the same transaction twice
	// dispatches nothing the second time. A rollback needs no compensation: the
	// rows went away with the transaction.
	if !committed || len(pending) == 0 || o.pool == nil {
		return 0
	}
	records, err := o.claimBySequence(ctx, pending, DefaultDrainLease)
	if err != nil {
		o.count(false)
		return 0
	}
	return o.dispatch(ctx, records)
}

// outboxRecord is one claimed row on its way to the receiver.
type outboxRecord struct {
	sequence int64
	item     DispatchItem
	attempts int
}

// claimBySequence takes the in-place path: only the rows this transaction
// registered, and only those a drainer has not already taken. SKIP LOCKED means
// a concurrent drainer and this call never process the same row.
func (o *PostgresOutbox) claimBySequence(ctx context.Context, sequences []int64, lease time.Duration) ([]outboxRecord, error) {
	return o.claim(ctx,
		`WITH due AS (
		   SELECT sequence FROM content_dispatch_outbox
		   WHERE sequence = ANY($1)
		     AND delivered_at IS NULL AND dead_lettered_at IS NULL
		     AND (claimed_until IS NULL OR claimed_until <= now())
		   FOR UPDATE SKIP LOCKED
		 )
		 UPDATE content_dispatch_outbox o SET claimed_until = now() + $2::interval, updated_at = now()
		 FROM due WHERE o.sequence = due.sequence
		 RETURNING o.sequence, o.item_id, o.kind, o.payload, o.idempotency_key, o.workspace_id, o.attempt_count`,
		sequences, lease.String())
}

// claimDue takes the sweep path: anything due, from any process, oldest first.
// This is what makes a restart recover — the in-place path only ever knows
// about rows its own process registered.
func (o *PostgresOutbox) claimDue(ctx context.Context, batch int, lease time.Duration) ([]outboxRecord, error) {
	return o.claim(ctx,
		`WITH due AS (
		   SELECT sequence FROM content_dispatch_outbox
		   WHERE delivered_at IS NULL AND dead_lettered_at IS NULL
		     AND next_attempt_at <= now()
		     AND (claimed_until IS NULL OR claimed_until <= now())
		   ORDER BY next_attempt_at, created_at
		   FOR UPDATE SKIP LOCKED
		   LIMIT $1
		 )
		 UPDATE content_dispatch_outbox o SET claimed_until = now() + $2::interval, updated_at = now()
		 FROM due WHERE o.sequence = due.sequence
		 RETURNING o.sequence, o.item_id, o.kind, o.payload, o.idempotency_key, o.workspace_id, o.attempt_count`,
		batch, lease.String())
}

func (o *PostgresOutbox) claim(ctx context.Context, query string, args ...any) ([]outboxRecord, error) {
	rows, err := o.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	claimed := []outboxRecord{}
	for rows.Next() {
		var r outboxRecord
		if err = rows.Scan(&r.sequence, &r.item.ID, &r.item.Kind, &r.item.Payload, &r.item.IdempotencyKey, &r.item.Workspace, &r.attempts); err != nil {
			return nil, err
		}
		claimed = append(claimed, r)
	}
	return claimed, rows.Err()
}

// dispatch delivers each claimed record and records the outcome. One failure
// never stops the others, and never reaches the caller: the business
// transaction has already committed, so reporting it as failed now would be a
// lie about an operation that succeeded.
func (o *PostgresOutbox) dispatch(ctx context.Context, records []outboxRecord) int {
	delivered := 0
	for _, r := range records {
		if o.Deliver != nil {
			if err := o.Deliver(ctx, r.item); err != nil {
				o.fail(ctx, r, err)
				continue
			}
		}
		if err := o.markDelivered(ctx, r.sequence); err != nil {
			// Delivered but not marked. The next sweep will deliver it again -
			// this is the window the receiver's idempotency has to absorb.
			o.count(false)
			continue
		}
		delivered++
	}
	return delivered
}

func (o *PostgresOutbox) markDelivered(ctx context.Context, sequence int64) error {
	_, err := o.pool.Exec(ctx,
		`UPDATE content_dispatch_outbox SET delivered_at = now(), claimed_until = NULL, updated_at = now() WHERE sequence = $1`,
		sequence)
	return err
}

// fail records one failed delivery: count it where an operator already looks,
// back the record off, and dead letter it once it has had its attempts.
func (o *PostgresOutbox) fail(ctx context.Context, r outboxRecord, cause error) {
	o.count(false)
	attempts := r.attempts + 1
	if attempts >= o.limits.maxAttempts() {
		if _, err := o.pool.Exec(ctx,
			`UPDATE content_dispatch_outbox
			 SET attempt_count = $2, dead_lettered_at = now(), claimed_until = NULL, last_error = $3, updated_at = now()
			 WHERE sequence = $1`,
			r.sequence, attempts, safeToken(cause.Error())); err != nil {
			o.count(false)
			return
		}
		o.deadLettered(r, attempts)
		return
	}
	if _, err := o.pool.Exec(ctx,
		`UPDATE content_dispatch_outbox
		 SET attempt_count = $2, next_attempt_at = now() + $3::interval, claimed_until = NULL, last_error = $4, updated_at = now()
		 WHERE sequence = $1`,
		r.sequence, attempts, backoff(attempts).String(), safeToken(cause.Error())); err != nil {
		o.count(false)
	}
}

// deadLettered reports a record that will not be retried again. It goes into
// the technical log the package already keeps, not into a counter: reusing
// Errors would fold "failed once more" and "will never be retried" into one
// number, and those are the two things worth telling apart.
//
// Every field here has to survive Sanitize, which is an allowlist. ObjectType
// must be one of the known kinds, and Message is replaced by the error code's
// own text, so the discriminator is Step - a free-form token that Sanitize
// keeps. An invented ObjectType would silently arrive as "unknown".
const dispatchDeadLetterStep = "dispatch_dead_letter"

func (o *PostgresOutbox) deadLettered(r outboxRecord, attempts int) {
	if o.log == nil {
		return
	}
	now := time.Now().UTC()
	o.log.Append(Event{
		ID: NewID(), Occurred: now, Received: now,
		Workspace: r.item.Workspace, ObjectType: "diagnostics", ObjectID: r.item.ID,
		ActorKind: "system", Action: "cancel", Outcome: "failed", Severity: "error",
		Component: "diagnostics", Code: "INTERNAL", Attempt: attempts,
		Step: dispatchDeadLetterStep,
	})
}

// backoff grows with the attempt count and stops at a cap, so a receiver that
// is down for an hour is retried on a schedule instead of in a tight loop.
func backoff(attempts int) time.Duration {
	d := outboxBackoffBase
	for range attempts - 1 {
		d *= 2
		if d >= outboxBackoffCap {
			return outboxBackoffCap
		}
	}
	return d
}
