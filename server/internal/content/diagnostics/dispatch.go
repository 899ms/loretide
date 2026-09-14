package diagnostics

import (
	"context"
	"sync"
)

// Transactional dispatch: a record whose existence is decided by a business
// transaction, but which can only be sent once that transaction has committed.
//
// The two halves this sits between already exist and are unchanged. Store.Audit
// writes inside the transaction and fails the whole thing when it cannot — that
// is what makes "a failure never produces a successful-looking audit" true, and
// critical audit writes must keep going through it, not through here. Store
// .Technical writes outside any transaction and only counts its failures, so a
// log sink cannot take a request down. Neither one can express "must not exist
// if the transaction rolls back, must be sent once it commits".
//
// LIMIT, DELIBERATE: this implementation is in process. A process that exits
// between commit and dispatch loses the record, and nothing outside the process
// ever knew about it. The specification records that as an accepted limit
// (FR-010a) rather than an oversight, and TestOutboxDoesNotSurviveTheProcess
// asserts it so that a durable implementation has a test to turn green. A
// durable version — a table written inside the same transaction and drained
// after it — is a separate task; the interface below is shaped so it can
// replace this one without any caller changing.

// DispatchItem is one record awaiting dispatch. The four parts are what a
// durable implementation needs: an id to address it, a kind to route it, the
// payload, and an idempotency key so a retry cannot take effect twice.
type DispatchItem struct {
	ID             string
	Kind           string
	Payload        []byte
	IdempotencyKey string
}

// Outbox stages records against a transaction and settles them with it.
//
// tx identifies the transaction the record belongs to. It is an opaque handle
// rather than a pgx.Tx so a caller can pass whatever its storage layer uses,
// and so this contract can be exercised without a database.
type Outbox interface {
	// Register stages a record. It must not be dispatched before Settle.
	Register(ctx context.Context, tx any, item DispatchItem) error
	// Settle resolves everything staged for tx and returns how many records
	// were dispatched. committed false discards them; committed true dispatches
	// them. Settling a transaction twice dispatches nothing the second time.
	Settle(ctx context.Context, tx any, committed bool) int
}

// MemoryOutbox is the in-process implementation. Failures and overflow are
// counted into the buffer the rest of the package already reports through
// Overview, so they surface where an operator is already looking rather than
// behind a new metric name.
type MemoryOutbox struct {
	// Deliver sends one record. Nil means dispatch is a no-op that still
	// settles, which is what an installation with no receiver wired should do.
	Deliver func(context.Context, DispatchItem) error

	mu       sync.Mutex
	staged   map[any][]DispatchItem
	seen     map[string]bool
	log      *LogBuffer
	capacity int
}

// NewMemoryOutbox returns an outbox that stages at most capacity records per
// transaction. log receives the failure and overflow counts.
func NewMemoryOutbox(log *LogBuffer, capacity int) *MemoryOutbox {
	if capacity < 1 {
		capacity = 1
	}
	return &MemoryOutbox{staged: map[any][]DispatchItem{}, seen: map[string]bool{}, log: log, capacity: capacity}
}

func (o *MemoryOutbox) count(drop bool) {
	if o.log == nil {
		return
	}
	if drop {
		o.log.Dropped.Add(1)
		return
	}
	o.log.Errors.Add(1)
}

// Register stages item against tx. Beyond capacity the record is dropped and
// counted: an unbounded queue would turn a slow receiver into a memory leak,
// and a silent drop would hide it.
func (o *MemoryOutbox) Register(_ context.Context, tx any, item DispatchItem) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.staged[tx]) >= o.capacity {
		o.count(true)
		return nil
	}
	o.staged[tx] = append(o.staged[tx], item)
	return nil
}

// Settle discards or dispatches everything staged for tx. Staging is cleared
// either way, so a second settle of the same transaction is a no-op and a
// rolled-back record cannot resurface on a later one.
func (o *MemoryOutbox) Settle(ctx context.Context, tx any, committed bool) int {
	o.mu.Lock()
	pending := o.staged[tx]
	delete(o.staged, tx)
	if !committed {
		o.mu.Unlock()
		return 0
	}
	send := make([]DispatchItem, 0, len(pending))
	for _, d := range pending {
		if d.IdempotencyKey != "" && o.seen[d.IdempotencyKey] {
			continue
		}
		if d.IdempotencyKey != "" {
			o.seen[d.IdempotencyKey] = true
		}
		send = append(send, d)
	}
	deliver := o.Deliver
	o.mu.Unlock()

	dispatched := 0
	for _, d := range send {
		if deliver == nil {
			dispatched++
			continue
		}
		if err := deliver(ctx, d); err != nil {
			// The caller's transaction already committed. Failing here would
			// report a business operation as failed after it succeeded, so the
			// failure is counted and the caller carries on.
			o.count(false)
			continue
		}
		dispatched++
	}
	return dispatched
}

// Pending reports how many records are staged but not yet settled. Intended for
// tests and for an operator asking whether anything is stuck.
func (o *MemoryOutbox) Pending() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := 0
	for _, staged := range o.staged {
		n += len(staged)
	}
	return n
}
