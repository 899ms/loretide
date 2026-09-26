package topicplanning

import "context"

// What search suggestions need to know about a work's documents (specs/036
// PR 2, contract §7.3). topic-planning does not import work-editor (FR-101):
// this small interface uses only this module's types and strings, and
// handler/content_search_suggestions.go answers it over work-editor's public
// Store.
//
// Every method answers ErrNotFound when the work, document or version is not
// there - missing, another brand's, or gone with a deleted workspace - and
// ErrStorage for anything else (FR-104). PR 2 has reads only; PR 3 adds the
// one write, Apply, used by adoption and by nothing else.

// SearchDocument is what a suggestion needs to know about its document.
type SearchDocument struct {
	WorkID     string
	ArtifactID string
	// LatestVersionID is "" when the document has no version.
	LatestVersionID string
	DraftSaved      bool
}

// SearchWorks is implemented by handler/content_search_suggestions.go.
type SearchWorks interface {
	Document(ctx context.Context, workspaceID, actor, workID, artifactID string) (SearchDocument, error)
	VersionBody(ctx context.Context, workspaceID, actor, workID, artifactID, versionID string) (string, error)
}
