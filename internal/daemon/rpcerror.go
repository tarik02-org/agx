package daemon

import (
	"errors"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
)

// codeReasons is the reason an error carries unless its site names one: the meaning docs/development/api.md
// gives each Connect code the daemon uses.
var codeReasons = map[connect.Code]v1.ErrorDetail_Reason{
	connect.CodeInvalidArgument:    v1.ErrorDetail_REASON_INVALID_REQUEST,
	connect.CodeNotFound:           v1.ErrorDetail_REASON_NOT_FOUND,
	connect.CodeAlreadyExists:      v1.ErrorDetail_REASON_CONFLICT,
	connect.CodeAborted:            v1.ErrorDetail_REASON_CONFLICT,
	connect.CodeResourceExhausted:  v1.ErrorDetail_REASON_CONFLICT,
	connect.CodeFailedPrecondition: v1.ErrorDetail_REASON_FAILED_PRECONDITION,
	connect.CodeUnavailable:        v1.ErrorDetail_REASON_UNAVAILABLE,
	connect.CodeUnauthenticated:    v1.ErrorDetail_REASON_UNAUTHENTICATED,
	connect.CodeDeadlineExceeded:   v1.ErrorDetail_REASON_TIMEOUT,
	connect.CodeInternal:           v1.ErrorDetail_REASON_INTERNAL,
}

// rpcError is a Connect error with the reason its code implies.
func rpcError(code connect.Code, message string) error {
	reason, ok := codeReasons[code]
	if !ok {
		reason = v1.ErrorDetail_REASON_INTERNAL
	}
	return reasonError(code, reason, message)
}

// reasonError is a Connect error whose reason is narrower than its code's, such as a failed
// precondition that a login resolves.
func reasonError(code connect.Code, reason v1.ErrorDetail_Reason, message string) *connect.Error {
	return detailError(code, &v1.ErrorDetail{Reason: reason}, message)
}

// configInvalid reports a configuration that does not load, with where err locates the problem.
func configInvalid(message string, err error) *connect.Error {
	detail := &v1.ErrorDetail{Reason: v1.ErrorDetail_REASON_CONFIG_INVALID}
	var located *config.SourceError
	if errors.As(err, &located) {
		detail.Source = &v1.SourceLocation{Path: located.Path, Line: uint32(located.Line)} //nolint:gosec // lines fit
	}
	return detailError(connect.CodeFailedPrecondition, detail, message)
}

func detailError(code connect.Code, detail *v1.ErrorDetail, message string) *connect.Error {
	err := connect.NewError(code, errors.New(message))
	d, detailErr := connect.NewErrorDetail(detail)
	if detailErr != nil {
		// Marshaling a reason and a location cannot fail.
		panic(detailErr)
	}
	err.AddDetail(d)
	return err
}
