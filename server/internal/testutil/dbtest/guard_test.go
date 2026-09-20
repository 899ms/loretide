package dbtest

import (
	"errors"
	"strings"
	"testing"
)

// LoadRequired is pure, so every rule it enforces is testable without a
// database — which matters more here than usual: this is the code that decides
// whether a database is touched at all.

const safeURL = "postgres://loretide_role:secret@127.0.0.1:5432/loretide_db?sslmode=disable"

func completeEnv(t *testing.T) {
	t.Helper()
	t.Setenv(EnvEnabled, "1")
	t.Setenv(EnvDatabaseURL, safeURL)
	t.Setenv(EnvDatabase, "loretide_db")
	t.Setenv(EnvRole, "loretide_role")
	t.Setenv(EnvRunID, "12345_1")
	t.Setenv(EnvSuite, SuiteHandler)
}

func TestACompleteConfigurationLoads(t *testing.T) {
	completeEnv(t)

	cfg, err := LoadRequired(SuiteHandler)
	if err != nil {
		t.Fatalf("a complete configuration was refused: %v", err)
	}
	if cfg.Database != "loretide_db" || cfg.Role != "loretide_role" || cfg.Suite != SuiteHandler {
		t.Errorf("loaded %+v", cfg)
	}
}

// The default case. Somebody ran `go test ./...`; there is no opt-in, so there
// is no database, and there is no skip either.
func TestWithoutTheOptInNothingLoads(t *testing.T) {
	completeEnv(t)
	t.Setenv(EnvEnabled, "")

	if _, err := LoadRequired(SuiteHandler); !errors.Is(err, ErrNotEnabled) {
		t.Fatalf("got %v, want ErrNotEnabled", err)
	}
}

// A value other than exactly "1" is not an opt-in. "0", "false" and "true" are
// all things people type when they mean different things.
func TestOnlyTheExactOptInValueCounts(t *testing.T) {
	for _, value := range []string{"0", "true", "yes", "TRUE", " 1", "1 "} {
		t.Run(value, func(t *testing.T) {
			completeEnv(t)
			t.Setenv(EnvEnabled, value)
			if _, err := LoadRequired(SuiteHandler); !errors.Is(err, ErrNotEnabled) {
				t.Errorf("%q was accepted as an opt-in: %v", value, err)
			}
		})
	}
}

// The generic DATABASE_URL must never be consulted. If it were, every
// developer shell with one exported would silently become an opt-in — which is
// exactly the failure this package exists to remove.
func TestTheGenericDatabaseURLIsNeverRead(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://multica:multica@127.0.0.1:5432/multica?sslmode=disable")
	t.Setenv(EnvEnabled, "1")
	t.Setenv(EnvDatabase, "loretide_db")
	t.Setenv(EnvRole, "loretide_role")
	t.Setenv(EnvRunID, "12345_1")
	t.Setenv(EnvSuite, SuiteHandler)
	t.Setenv(EnvDatabaseURL, "")

	_, err := LoadRequired(SuiteHandler)
	if !errors.Is(err, ErrIncomplete) {
		t.Fatalf("got %v, want ErrIncomplete — the generic URL must not stand in", err)
	}
	if strings.Contains(err.Error(), "multica") {
		t.Errorf("the generic URL leaked into the error: %v", err)
	}
}

func TestEveryRequiredVariableIsRequired(t *testing.T) {
	for _, name := range []string{EnvDatabaseURL, EnvDatabase, EnvRole, EnvRunID, EnvSuite} {
		t.Run(name, func(t *testing.T) {
			completeEnv(t)
			t.Setenv(name, "")
			if _, err := LoadRequired(SuiteHandler); !errors.Is(err, ErrIncomplete) {
				t.Errorf("got %v, want ErrIncomplete", err)
			}
		})
	}
}

// A whitespace-only value is not a value. It is what a shell produces from an
// unset variable inside quotes.
func TestBlankValuesAreNotValues(t *testing.T) {
	completeEnv(t)
	t.Setenv(EnvDatabase, "   ")

	if _, err := LoadRequired(SuiteHandler); !errors.Is(err, ErrIncomplete) {
		t.Errorf("got %v, want ErrIncomplete", err)
	}
}

// The suite is fixed by the code and compared with the environment, so a
// handler command that inherited a cmd-server environment is caught rather
// than run against the other suite's database.
func TestTheSuiteMustMatchTheBinary(t *testing.T) {
	completeEnv(t)
	t.Setenv(EnvSuite, SuiteCmdServer)

	if _, err := LoadRequired(SuiteHandler); !errors.Is(err, ErrSuiteMismatch) {
		t.Fatalf("got %v, want ErrSuiteMismatch", err)
	}

	t.Setenv(EnvSuite, SuiteHandler)
	if _, err := LoadRequired(SuiteCmdServer); !errors.Is(err, ErrSuiteMismatch) {
		t.Fatalf("got %v, want ErrSuiteMismatch", err)
	}
}

func TestAnUnknownSuiteNameIsRefused(t *testing.T) {
	completeEnv(t)
	t.Setenv(EnvSuite, "handler")

	if _, err := LoadRequired("something-else"); !errors.Is(err, ErrSuiteMismatch) {
		t.Errorf("got %v, want ErrSuiteMismatch", err)
	}
}

func TestIdentifiersMustBePlain(t *testing.T) {
	for name, value := range map[string]string{
		"a quoted name":       `"loretide db"`,
		"a name with a space": "loretide db",
		"uppercase":           "Loretide_DB",
		"a semicolon":         "loretide_db;drop",
		"empty after trim":    " ",
		"leading digit":       "1loretide",
	} {
		t.Run(name, func(t *testing.T) {
			completeEnv(t)
			t.Setenv(EnvDatabase, value)
			if _, err := LoadRequired(SuiteHandler); !errors.Is(err, ErrIncomplete) {
				t.Errorf("%q was accepted as a database name: %v", value, err)
			}
		})
	}
}

// A9 of the URL contract: anything that can change which server or database is
// reached makes the name comparison meaningless, because the comparison was
// made against the name the caller supplied.
func TestRoutingOverridesInTheURLAreRefused(t *testing.T) {
	for name, query := range map[string]string{
		"host":                 "host=evil.example",
		"hostaddr":             "hostaddr=10.0.0.1",
		"port":                 "port=6432",
		"user":                 "user=postgres",
		"dbname":               "dbname=multica",
		"database":             "database=multica",
		"service":              "service=prod",
		"servicefile":          "servicefile=/tmp/pgservice",
		"target_session_attrs": "target_session_attrs=any",
		"uppercase host":       "HOST=evil.example",
	} {
		t.Run(name, func(t *testing.T) {
			completeEnv(t)
			t.Setenv(EnvDatabaseURL, safeURL+"&"+query)
			if _, err := LoadRequired(SuiteHandler); !errors.Is(err, ErrUnsafeURL) {
				t.Errorf("%q was accepted: %v", query, err)
			}
		})
	}
}

// An unreviewed parameter is refused rather than ignored: libpq understands a
// long list, and the one nobody considered is the one that matters.
func TestAnUnreviewedQueryParameterIsRefused(t *testing.T) {
	completeEnv(t)
	t.Setenv(EnvDatabaseURL, safeURL+"&options=-c%20search_path%3Devil")

	if _, err := LoadRequired(SuiteHandler); !errors.Is(err, ErrUnsafeURL) {
		t.Errorf("got %v, want ErrUnsafeURL", err)
	}
}

func TestReviewedQueryParametersAreAccepted(t *testing.T) {
	completeEnv(t)
	t.Setenv(EnvDatabaseURL, safeURL+"&connect_timeout=5&application_name=loretide-tests")

	if _, err := LoadRequired(SuiteHandler); err != nil {
		t.Errorf("a reviewed parameter was refused: %v", err)
	}
}

func TestUnsafeURLShapesAreRefused(t *testing.T) {
	for name, raw := range map[string]string{
		"multiple hosts":         "postgres://r:p@a.example,b.example:5432/loretide_db",
		"a unix socket":          "postgres://r:p@/loretide_db",
		"an encoded socket path": "postgres://r:p@%2Fvar%2Frun%2Fpostgresql/loretide_db",
		"no database":            "postgres://r:p@127.0.0.1:5432/",
		"a mysql URL":            "mysql://r:p@127.0.0.1:3306/loretide_db",
		"not a URL":              "://::::",
	} {
		t.Run(name, func(t *testing.T) {
			completeEnv(t)
			t.Setenv(EnvDatabaseURL, raw)
			if _, err := LoadRequired(SuiteHandler); !errors.Is(err, ErrUnsafeURL) {
				t.Errorf("%q was accepted: %v", raw, err)
			}
		})
	}
}

// Every rejection reason is printed; none of them may print the password.
func TestNoErrorEverCarriesTheCredential(t *testing.T) {
	const password = "correct-horse-battery-staple"
	withSecret := "postgres://loretide_role:" + password + "@127.0.0.1:5432/loretide_db?sslmode=disable&options=x"

	completeEnv(t)
	t.Setenv(EnvDatabaseURL, withSecret)
	_, err := LoadRequired(SuiteHandler)
	if err == nil {
		t.Fatal("the control case must fail, or this test proves nothing")
	}
	if strings.Contains(err.Error(), password) {
		t.Errorf("the password reached an error message: %v", err)
	}

	// And the same for the unparseable case, where a parser error would
	// otherwise echo the input.
	t.Setenv(EnvDatabaseURL, "postgres://loretide_role:"+password+"@127.0.0.1:5432/db\x7f")
	if _, err := LoadRequired(SuiteHandler); err != nil && strings.Contains(err.Error(), password) {
		t.Errorf("the password reached a parser error: %v", err)
	}
}

func TestDescribeNamesTheTargetWithoutTheURL(t *testing.T) {
	cfg := Config{
		DatabaseURL: safeURL, Database: "loretide_db", Role: "loretide_role",
		RunID: "12345_1", Suite: SuiteHandler,
	}

	described := cfg.Describe()
	for _, want := range []string{"loretide_db", "loretide_role", "12345_1", SuiteHandler} {
		if !strings.Contains(described, want) {
			t.Errorf("Describe() omitted %q: %s", want, described)
		}
	}
	if strings.Contains(described, "secret") || strings.Contains(described, "127.0.0.1") {
		t.Errorf("Describe() leaked connection detail: %s", described)
	}
}
