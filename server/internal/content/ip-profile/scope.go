package ipprofile

import (
	"errors"
	"maps"
	"slices"
)

// The account's material scope preference: which sources a run starts from.
//
// This is "what was chosen last time", not a configuration version. The persona
// prompt needs a version history because a run pins one (LT-012); this needs
// none, because every run already records the preference it saw in its own
// snapshot (diagnostics.Snapshot.Preference).
//
// It lives at settings["loretide.scope"] rather than in a column, so there is
// no migration and no change to the account table. The shape is taken from the
// brand workspace's timezone (specs/004): validate on write, fill the default
// on read, and never let a read write.
//
// Contract: specs/018-lt014-account-scope-preference/contracts/account-scope.md

// Scope is where a run may draw material from.
type Scope string

const (
	// ScopeLocal uses only material already held locally.
	ScopeLocal Scope = "local"
	// ScopeWeb uses only material retrieved from the web.
	ScopeWeb Scope = "web"
	// ScopeAll uses both.
	ScopeAll Scope = "all"
)

// Scopes is the controlled set. Mirrored by packages/core/content/ip-profile/
// scope.ts, which reads this file and compares rather than restating it.
var Scopes = []Scope{ScopeLocal, ScopeWeb, ScopeAll}

// ScopeSettingsKey is where the preference lives inside the account's settings
// JSONB. The "loretide." prefix keeps it clear of any upstream key called
// "scope", the same reason the workspace timezone carries one.
const ScopeSettingsKey = "loretide.scope"

// DefaultScope is what an account reads as when it has never chosen - including
// every account created before this feature. Never the empty string: the start
// screen renders a radio group from this value, and an empty one would leave
// all three unselected on first open.
const DefaultScope = string(ScopeAll)

// ErrScope is an unrecognised scope. Answered as 400 with a diagnostic error
// object, never as a storage error.
var ErrScope = errors.New("unsupported material scope")

// ValidateScope accepts only an exact match: no trimming, no case folding.
// Tolerating "All" alongside "all" would give one preference two spellings in
// storage, and every reader would then have to know both.
func ValidateScope(value string) error {
	if !slices.Contains(Scopes, Scope(value)) {
		return ErrScope
	}
	return nil
}

// ScopeOf reads the preference off a settings blob, falling back to the default
// for anything unreadable: settings that are absent, not an object, missing the
// key, or holding a value nothing recognises.
//
// It does NOT write the default back, and callers must not either. An account
// that never chose and one that chose "all" deliberately would otherwise become
// the same row, and the first read would have made that choice for the person.
func ScopeOf(settings map[string]any) string {
	raw, present := settings[ScopeSettingsKey]
	if !present {
		return DefaultScope
	}
	value, isString := raw.(string)
	if !isString || ValidateScope(value) != nil {
		return DefaultScope
	}
	return value
}

// withScope returns a NEW settings map carrying the scope, leaving the caller's
// untouched.
//
// Merging is not an optimisation. The update query replaces the settings blob
// wholesale (handler/content_account.go marshals the patch straight over the
// stored value), so writing only this key would delete every other setting the
// account has. Doing it here means no call site has to remember that.
func withScope(settings map[string]any, scope string) map[string]any {
	merged := make(map[string]any, len(settings)+1)
	maps.Copy(merged, settings)
	merged[ScopeSettingsKey] = scope
	return merged
}

// scopeFilled returns settings with the scope key present, filling the default
// when it is absent or unusable. Applied on the way OUT only: the stored row
// keeps whatever it had, and only the response is complete.
func scopeFilled(settings map[string]any) map[string]any {
	if existing, present := settings[ScopeSettingsKey]; present {
		if value, isString := existing.(string); isString && ValidateScope(value) == nil {
			return settings
		}
	}
	return withScope(settings, DefaultScope)
}

// validateScopeSetting rejects a settings blob whose scope key is unusable.
// Settings without the key are fine: the preference is optional at creation,
// and an unrelated update must not be forced to carry one.
//
// This runs on the general update path as well as the dedicated endpoint. With
// only the endpoint checking, anyone could put a value the readers refuse into
// the row through PATCH, and the endpoint's validation would be decoration.
func validateScopeSetting(settings map[string]any) error {
	raw, present := settings[ScopeSettingsKey]
	if !present {
		return nil
	}
	value, isString := raw.(string)
	if !isString {
		return ErrScope
	}
	return ValidateScope(value)
}
