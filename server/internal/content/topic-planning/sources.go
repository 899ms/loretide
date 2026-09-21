package topicplanning

import (
	"context"
	"strings"
)

// MaxSourceReferencesPerField is the maximum number of source references
// permitted per card reference field (fit_source_ids or evidence_source_ids).
const MaxSourceReferencesPerField = 50

// SourceReader answers whether a source id belongs to this brand.
//
// Strings only, no source-inbox types: topic-planning's dependency row does
// not list source-inbox (the path through knowledge-base is two hops), and a
// port that carried sourceinbox.ErrNotFound would make this module import it.
// The adapter in handler/content_topic.go does the mapping instead.
type SourceReader interface {
	Exists(ctx context.Context, workspaceID, sourceID string) (bool, error)
}

// NormalizeSourceIDs cleans, deduplicates, and limits a source ID reference list.
//
// Three operations in order:
// 1. Trim whitespace: empty or whitespace-only IDs are discarded.
// 2. Deduplicate: preserves first-seen order.
// 3. Upper limit: capped at MaxSourceReferencesPerField (50). If exceeded,
//    returns a FieldError naming the field.
//
// We explicitly do NOT touch or reuse normalizeStrings: normalizeStrings only
// converts nil to [] for backward compatibility with channels (022), and modifying
// it would change 022's behavior.
func NormalizeSourceIDs(field string, values []string) ([]string, error) {
	if values == nil {
		return []string{}, nil
	}
	cleaned := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))

	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		cleaned = append(cleaned, trimmed)
	}

	if len(cleaned) > MaxSourceReferencesPerField {
		return nil, FieldError{
			Field:  field,
			Reason: "too many source references",
		}
	}

	return cleaned, nil
}
