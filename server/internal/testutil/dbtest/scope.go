package dbtest

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// SuiteScope names one run of one suite.
//
// The two suites used to seed a root user and workspace under fixed names, and
// to clean up by deleting every row matching those names. Two problems:
// concurrent runs against one database collided, and a delete by email or slug
// reaches rows this run never created.
//
// The names below are unique per run so they can be read in a psql session
// without ambiguity. They are NOT the cleanup key: deletion goes by the ids the
// inserts returned. A name is a label; an id is an identity.
type SuiteScope struct {
	RunID  string
	Suite  string
	Unique string
}

// NewSuiteScope mixes the validated run identity with a fresh random component.
// The run id alone is not enough: a rerun of the same CI attempt, or two
// packages sharing one database, would produce the same names again.
func NewSuiteScope(cfg Config) SuiteScope {
	var noise [6]byte
	if _, err := rand.Read(noise[:]); err != nil {
		// A test process that cannot read randomness cannot promise unique
		// fixtures, and running with colliding names is the failure this
		// exists to prevent.
		panic("dbtest: cannot generate a suite scope: " + err.Error())
	}
	return SuiteScope{RunID: cfg.RunID, Suite: cfg.Suite, Unique: hex.EncodeToString(noise[:])}
}

// Email is a unique address for a root fixture user. The domain is reserved for
// documentation by RFC 2606, so nothing here can be delivered anywhere.
func (s SuiteScope) Email(prefix string) string {
	return fmt.Sprintf("%s-%s-%s@tests.invalid", prefix, s.Suite, s.Unique)
}

// Slug is a unique workspace slug. Lowercase hex and dashes only, so it stays
// inside whatever the slug column accepts.
func (s SuiteScope) Slug(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, s.Unique)
}

// String is for diagnostics: it names the run without naming a target.
func (s SuiteScope) String() string {
	return fmt.Sprintf("suite=%s run=%s unique=%s", s.Suite, s.RunID, s.Unique)
}
