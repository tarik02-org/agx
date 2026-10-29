package daemon

import (
	"errors"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
)

func rpcError(code connect.Code, message string) *connect.Error {
	return connect.NewError(code, errors.New(message))
}

// configInvalid reports a configuration that does not load, with where err locates the problem.
func configInvalid(message string, err error) *connect.Error {
	detail := &v1.ErrorDetail{}
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
		// These generated diagnostics contain no unsupported values.
		panic(detailErr)
	}
	err.AddDetail(d)
	return err
}
