package handler

import "net/http"

// A document's version history (specs/024).
//
// There is no DELETE here and no PATCH of a version. That is not an omission:
// SOP 7.1 requires approved, handed over and published versions to be kept, and
// the way to keep them is for no path to exist that could remove one.

// SaveContentArtifactVersion stores the editing copy as a new version.
func (h *Handler) SaveContentArtifactVersion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.workScope(w, r)
	if !ok {
		return
	}
	version, err := h.workEditorStore().SaveVersion(r.Context(), workspace, actor,
		workIDFromURL(r), artifactIDFromURL(r))
	if err != nil {
		h.workError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, version)
}

// ImportContentArtifactVersion stores the one version of a document imported
// from something already published (SOP 3.3).
//
// A separate endpoint rather than a flag on the save endpoint, for the reason
// the two metric endpoints in 027 are separate: which entry point was called
// is what the server records as the version's provenance, and a caller that
// could declare its own provenance is not reporting one.
func (h *Handler) ImportContentArtifactVersion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.workScope(w, r)
	if !ok {
		return
	}
	version, err := h.workEditorStore().ImportVersion(r.Context(), workspace, actor,
		workIDFromURL(r), artifactIDFromURL(r))
	if err != nil {
		h.workError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, version)
}

func (h *Handler) ListContentArtifactVersions(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.workScope(w, r)
	if !ok {
		return
	}
	versions, err := h.workEditorStore().ListVersions(r.Context(), workspace, actor,
		workIDFromURL(r), artifactIDFromURL(r))
	if err != nil {
		h.workError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
}

func (h *Handler) GetContentArtifactVersion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.workScope(w, r)
	if !ok {
		return
	}
	version, err := h.workEditorStore().GetVersion(r.Context(), workspace, actor,
		workIDFromURL(r), artifactIDFromURL(r), versionIDFromURL(r))
	if err != nil {
		h.workError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, version)
}

// RestoreContentArtifactVersion appends a version carrying an older one's text.
// It does not rewind: the history stays whole, and the new version records that
// it came from a restore.
func (h *Handler) RestoreContentArtifactVersion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.workScope(w, r)
	if !ok {
		return
	}
	version, err := h.workEditorStore().RestoreVersion(r.Context(), workspace, actor,
		workIDFromURL(r), artifactIDFromURL(r), versionIDFromURL(r))
	if err != nil {
		h.workError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, version)
}

// AdoptContentArtifactVersion marks a version as the next step's baseline, as a
// new version rather than a mutable pointer - a pointer would lose the record
// of which versions were ever adopted.
func (h *Handler) AdoptContentArtifactVersion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.workScope(w, r)
	if !ok {
		return
	}
	version, err := h.workEditorStore().AdoptVersion(r.Context(), workspace, actor,
		workIDFromURL(r), artifactIDFromURL(r), versionIDFromURL(r))
	if err != nil {
		h.workError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, version)
}
