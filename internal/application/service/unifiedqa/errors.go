package unifiedqa

import "errors"

var (
	ErrNoAccessibleKnowledgeBase = errors.New("no accessible knowledge base")
	ErrRouteInvalidModelOutput   = errors.New("route model returned invalid output")
)

const (
	ErrorCodeNodeExecutionFailed = "NODE_EXECUTION_FAILED"
	ErrorCodeContextCancelled    = "CONTEXT_CANCELLED"
	ErrorCodeContextDeadline     = "CONTEXT_DEADLINE_EXCEEDED"
	ErrorCodeRouteUnavailable    = "ROUTE_UNAVAILABLE"
	ErrorCodeRouteModelFailed    = "ROUTE_MODEL_FAILED"
	ErrorCodeRouteInvalidOutput  = "ROUTE_INVALID_OUTPUT"
)

// ErrorCoder lets workflow components persist stable diagnostic codes without
// changing the business error returned to their caller.
type ErrorCoder interface {
	ErrorCode() string
}
