package handler

import (
	"testing"
)

// TestModelCatalogCacheDecision pins the three-way branch ReportModelListResult
// takes on a completed report.
//
// The fallback row is the MUL-5549 regression. A static stand-in is non-empty
// and `supported`, so it used to be indistinguishable from a real catalog and
// got stored as last-known-good for the full 24h serve window. It must now be
// Keep: not stored, but also not allowed to evict a real catalog we already
// hold — a stand-in is no evidence about what the runtime supports.
func TestModelCatalogCacheDecision(t *testing.T) {
	for _, tc := range []struct {
		name      string
		models    []ModelEntry
		supported bool
		fallback  bool
		want      modelCatalogCacheAction
	}{
		{
			name:      "real non-empty catalog is stored",
			models:    sampleCatalog(),
			supported: true,
			want:      modelCatalogCacheStore,
		},
		{
			name:      "fallback catalog leaves the cache alone",
			models:    sampleCatalog(),
			supported: true,
			fallback:  true,
			want:      modelCatalogCacheKeep,
		},
		{
			name:      "authoritatively empty catalog drops the snapshot",
			models:    nil,
			supported: true,
			want:      modelCatalogCacheDrop,
		},
		{
			name:      "runtime that ignores model selection drops the snapshot",
			models:    sampleCatalog(),
			supported: false,
			want:      modelCatalogCacheDrop,
		},
		{
			name:      "an empty fallback still only keeps",
			models:    nil,
			supported: true,
			fallback:  true,
			want:      modelCatalogCacheKeep,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := modelCatalogCacheDecision(tc.models, tc.supported, tc.fallback)
			if got != tc.want {
				t.Errorf("modelCatalogCacheDecision = %v, want %v", got, tc.want)
			}
		})
	}
}
