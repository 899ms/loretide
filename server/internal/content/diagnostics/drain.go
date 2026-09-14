package diagnostics

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// Drainer is where a restart actually recovers. The in-place path in
// PostgresOutbox.Settle only knows about rows the current process registered;
// anything committed by a process that then exited is invisible to it and
// visible here.
//
// It is a goroutine inside the process that already serves diagnostics, not a
// separate service: one more ticker, not one more deployment unit.
type Drainer struct {
	out  *PostgresOutbox
	opts DrainerOptions
}

// DrainerOptions are all injected. The package holds defaults; the server
// wiring reads the environment and passes real values, and a test passes a
// short interval and a millisecond lease so it does not have to wait.
type DrainerOptions struct {
	// Interval between sweeps. The delay a stranded record can see is bounded
	// by this.
	Interval time.Duration
	// Lease is how long a claim holds a record. After it expires another
	// drainer may take the record — which is safe only because delivery is
	// expected to be idempotent on the receiving side.
	Lease time.Duration
	// Batch is how many records one sweep claims.
	Batch int
	// Retention is how long a delivered or dead-lettered record is kept before
	// the sweep deletes it. Zero means the package default.
	Retention time.Duration
}

func (o DrainerOptions) interval() time.Duration {
	if o.Interval > 0 {
		return o.Interval
	}
	return DefaultDrainInterval
}

func (o DrainerOptions) lease() time.Duration {
	if o.Lease > 0 {
		return o.Lease
	}
	return DefaultDrainLease
}

func (o DrainerOptions) batch() int {
	if o.Batch > 0 {
		return o.Batch
	}
	return DefaultDrainBatch
}

func (o DrainerOptions) retention() time.Duration {
	if o.Retention > 0 {
		return o.Retention
	}
	return 7 * 24 * time.Hour
}

// NewDrainer returns a drainer over out. It does nothing until Start or
// DrainOnce is called.
func NewDrainer(out *PostgresOutbox, opts DrainerOptions) *Drainer {
	return &Drainer{out: out, opts: opts}
}

// Start sweeps on an interval until ctx is cancelled. It returns once the sweep
// in progress has finished, so a shut-down process leaves no goroutine holding
// a claim.
func (d *Drainer) Start(ctx context.Context) {
	ticker := time.NewTicker(d.opts.interval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.DrainOnce(ctx)
		}
	}
}

// DrainOnce claims one batch of due records, delivers them, and prunes what has
// outlived the retention window. It returns how many records were delivered.
//
// Claiming is both SKIP LOCKED and a lease, and they buy different things. The
// lease is what makes two drainers safe: it is written to the row, so it
// survives the claiming connection and a second drainer re-reads the row after
// the first one committed and skips it. SKIP LOCKED is what keeps that second
// drainer from *waiting* for the first — without it a sweep blocks behind any
// transaction holding the row, and drainers serialise instead of sharing the
// batch. TestDrainerDoesNotBlockOnARowAnotherTransactionHolds is the only case
// that tells the two apart; removing SKIP LOCKED leaves every other test green.
func (d *Drainer) DrainOnce(ctx context.Context) int {
	if d.out == nil || d.out.pool == nil {
		return 0
	}
	records, err := d.out.claimDue(ctx, d.opts.batch(), d.opts.lease())
	if err != nil {
		d.out.count(false)
		return 0
	}
	delivered := d.out.dispatch(ctx, records)
	d.prune(ctx)
	return delivered
}

// prune removes records that are finished — delivered or dead lettered — and
// older than the retention window. It rides the same wake-up on purpose: two
// timers that do not know about each other are harder to reason about than one
// that does slightly more.
func (d *Drainer) prune(ctx context.Context) {
	if _, err := d.out.pool.Exec(ctx,
		`DELETE FROM content_dispatch_outbox
		 WHERE (delivered_at IS NOT NULL AND delivered_at < now() - $1::interval)
		    OR (dead_lettered_at IS NOT NULL AND dead_lettered_at < now() - $1::interval)`,
		d.opts.retention().String()); err != nil {
		d.out.count(false)
	}
}

// NewDispatch builds the durable outbox and its drainer from raw configuration
// strings, the same shape ConfigureLimits already uses: the server hands over
// what the environment said and this package decides what is valid. Invalid
// configuration is an error, never a silent default — a drainer that quietly
// fell back to a default interval would be indistinguishable from one that read
// the value the operator set.
//
// Empty strings mean "use the package default". This package never reads the
// environment itself.
func NewDispatch(store *Store, intervalSeconds, leaseSeconds, maxAttempts, maxPayloadKB string) (*PostgresOutbox, *Drainer, error) {
	limits := OutboxLimits{}
	opts := DrainerOptions{}
	if store != nil {
		opts.Retention = store.Retention
	}
	if err := assignDuration(intervalSeconds, "LORETIDE_DIAG_DISPATCH_INTERVAL_SECONDS", 1, 3600, &opts.Interval); err != nil {
		return nil, nil, err
	}
	if err := assignDuration(leaseSeconds, "LORETIDE_DIAG_DISPATCH_LEASE_SECONDS", 1, 3600, &opts.Lease); err != nil {
		return nil, nil, err
	}
	if err := assignInt(maxAttempts, "LORETIDE_DIAG_DISPATCH_MAX_ATTEMPTS", 1, 100, &limits.MaxAttempts); err != nil {
		return nil, nil, err
	}
	kb := 0
	if err := assignInt(maxPayloadKB, "LORETIDE_DIAG_DISPATCH_MAX_PAYLOAD_KB", 1, 65536, &kb); err != nil {
		return nil, nil, err
	}
	if kb > 0 {
		limits.MaxPayload = kb * 1024
	}
	if store == nil {
		return nil, nil, fmt.Errorf("diagnostics: dispatch needs a store")
	}
	out := NewPostgresOutbox(store.Pool(), store.Log, limits)
	return out, NewDrainer(out, opts), nil
}

func assignInt(raw, name string, low, high int, into *int) error {
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < low || n > high {
		return fmt.Errorf("%s must be %d..%d", name, low, high)
	}
	*into = n
	return nil
}

func assignDuration(raw, name string, low, high int, into *time.Duration) error {
	n := 0
	if err := assignInt(raw, name, low, high, &n); err != nil {
		return err
	}
	if n > 0 {
		*into = time.Duration(n) * time.Second
	}
	return nil
}
