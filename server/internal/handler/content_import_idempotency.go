package handler

import (
	"net/http"

	"github.com/multica-ai/multica/server/internal/content/idempotency"
	publicapiv1 "github.com/multica-ai/multica/server/pkg/publicapi/v1"
)

func historicalImportRequest(r *http.Request, operation, resource string, body any) (idempotency.Request, error) {
	return idempotency.NewRequest(operation, resource, r.Header.Get(publicapiv1.HeaderIdempotencyKey), body)
}

func optionalIdempotencyRequest(enabled bool, request idempotency.Request) []idempotency.Request {
	if !enabled {
		return nil
	}
	return []idempotency.Request{request}
}
