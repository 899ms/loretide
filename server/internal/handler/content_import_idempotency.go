package handler

import (
	"net/http"
	"strings"

	"github.com/multica-ai/multica/server/internal/content/idempotency"
	publicapiv1 "github.com/multica-ai/multica/server/pkg/publicapi/v1"
)

// historicalImportRequest opts a historical write into replay protection when
// the caller supplies an Idempotency-Key. The historical flag describes the
// record's provenance; it must not turn an existing editor caller without that
// header into a 400. New import adapters send a key and therefore get the
// replay/conflict contract, while compatible callers retain the ordinary write
// behaviour.
func historicalImportRequest(r *http.Request, operation, resource string, body any) (idempotency.Request, bool, error) {
	key := strings.TrimSpace(r.Header.Get(publicapiv1.HeaderIdempotencyKey))
	if key == "" {
		return idempotency.Request{}, false, nil
	}
	request, err := idempotency.NewRequest(operation, resource, key, body)
	return request, err == nil, err
}

func optionalIdempotencyRequest(enabled bool, request idempotency.Request) []idempotency.Request {
	if !enabled {
		return nil
	}
	return []idempotency.Request{request}
}
