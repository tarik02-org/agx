package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/configedit"
	"github.com/tarik02-org/agx/internal/daemon"
	"github.com/tarik02-org/agx/internal/packages"
)

// scripting is the --json decision of this invocation: the persistent --json flag, or for usage
// errors reported before cobra parses flags, wantsJSON. In JSON mode a command writes one
// document, or a login's stream of them, to stdout, never prompts, and reports errors as
// documents.
var scripting struct {
	enabled bool
	stdout  io.Writer
}

// wantsJSON finds --json among the arguments before any "--".
func wantsJSON(args []string) bool {
	end := slices.Index(args, "--")
	if end < 0 {
		end = len(args)
	}
	return slices.Contains(args[:end], "--json")
}

const jsonVersion = 1

// protoJSON prints daemon messages as the API defines them, every field present.
var protoJSON = protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}

func printJSON(value any) error {
	if m, ok := value.(proto.Message); ok {
		b, err := protoJSON.Marshal(m)
		if err != nil {
			return err
		}
		value = json.RawMessage(b)
	}
	return json.NewEncoder(scripting.stdout).Encode(struct {
		Version int `json:"version"`
		Data    any `json:"data"`
	}{jsonVersion, value})
}

// emit is the one output path of a command's result: data as a JSON document, or text for a
// person.
func emit(data any, text func() error) error {
	if scripting.enabled {
		return printJSON(data)
	}
	return text()
}

// done is the text of a result that a status line says all about.
func done(message string) func() error {
	return func() error {
		_, err := fmt.Fprintln(ui.Stdout, ui.Stdout.Status(ui.OK, message))
		return err
	}
}

// hint writes a note for a person; in JSON mode only a terminal on stderr gets it.
func hint(text string) {
	if scripting.enabled && !ui.Stderr.Terminal() {
		return
	}
	ui.Note(text)
}

// Error codes of JSON error documents. They are part of the output contract (docs/cli-reference.md, JSON output).
const (
	codeUsage              = "usage"
	codeConfigInvalid      = "config_invalid"
	codeNotFound           = "not_found"
	codeConflict           = "conflict"
	codeReadOnly           = "read_only"
	codeInvalidRequest     = "invalid_request"
	codeFailedPrecondition = "failed_precondition"
	codeNeedsLogin         = "needs_login"
	codeMCPFailed          = "mcp_failed"
	codeDaemonUnavailable  = "daemon_unavailable"
	codeVersionMismatch    = "version_mismatch"
	codeNetwork            = "network"
	codeTimeout            = "timeout"
	codeInterrupted        = "interrupted"
	codeInternal           = "internal"
)

// codedError classifies an error where it arises; the outermost code wins.
type codedError struct {
	code string
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }

func (e *codedError) Unwrap() error { return e.err }

func coded(code string, err error) error {
	if err == nil {
		return nil
	}
	return &codedError{code: code, err: err}
}

func codedf(code, format string, args ...any) error {
	return &codedError{code: code, err: fmt.Errorf(format, args...)}
}

// errorCode classifies err for a JSON error document; what nothing classifies is "internal".
func errorCode(err error) string {
	var usage *usageError
	var c *codedError
	var edited *configedit.Error
	var rpc *connect.Error
	var netErr net.Error
	switch {
	case errors.As(err, &usage):
		return codeUsage
	case errors.As(err, &c):
		return c.code
	case errors.As(err, &edited):
		return string(edited.Code)
	case errors.Is(err, daemon.ErrVersionMismatch):
		return codeVersionMismatch
	case errors.Is(err, daemon.ErrUnavailable):
		return codeDaemonUnavailable
	case errors.Is(err, packages.ErrFetch):
		return codeNetwork
	case errors.Is(err, packages.ErrStaleLock), errors.Is(err, packages.ErrBusy):
		return codeConflict
	case errors.Is(err, packages.ErrUnknown):
		return codeNotFound
	case errors.Is(err, auth.ErrNoCredential):
		return codeNeedsLogin
	case errors.As(err, &rpc):
		return connectCode(rpc)
	case errors.Is(err, context.DeadlineExceeded):
		return codeTimeout
	case errors.Is(err, context.Canceled):
		return codeInterrupted
	case errors.Is(err, fs.ErrNotExist):
		return codeNotFound
	case errors.Is(err, fs.ErrExist):
		return codeConflict
	case errors.Is(err, fs.ErrPermission):
		return codeReadOnly
	case errors.As(err, &netErr):
		return codeNetwork
	}
	return codeInternal
}

// reasonCodes maps the reasons daemon errors carry. A reason this build does not know is
// unclassified, so it maps to codeInternal like any defect.
var reasonCodes = map[v1.ErrorDetail_Reason]string{
	v1.ErrorDetail_REASON_INVALID_REQUEST:     codeInvalidRequest,
	v1.ErrorDetail_REASON_NOT_FOUND:           codeNotFound,
	v1.ErrorDetail_REASON_CONFLICT:            codeConflict,
	v1.ErrorDetail_REASON_FAILED_PRECONDITION: codeFailedPrecondition,
	v1.ErrorDetail_REASON_NEEDS_LOGIN:         codeNeedsLogin,
	v1.ErrorDetail_REASON_CONFIG_INVALID:      codeConfigInvalid,
	v1.ErrorDetail_REASON_UNAVAILABLE:         codeDaemonUnavailable,
	// The caller's credential belongs to a daemon that is no longer the one answering.
	v1.ErrorDetail_REASON_UNAUTHENTICATED: codeDaemonUnavailable,
	v1.ErrorDetail_REASON_TIMEOUT:         codeTimeout,
	v1.ErrorDetail_REASON_INTERNAL:        codeInternal,
	v1.ErrorDetail_REASON_MCP_FAILED:      codeMCPFailed,
	v1.ErrorDetail_REASON_NETWORK:         codeNetwork,
}

// connectCode classifies a Connect error by the reason the daemon attached. An error without one
// arose in the transport, on this side of the connection.
func connectCode(err *connect.Error) string {
	for _, d := range err.Details() {
		value, decodeErr := d.Value()
		detail, ok := value.(*v1.ErrorDetail)
		if decodeErr != nil || !ok {
			continue
		}
		if code, known := reasonCodes[detail.Reason]; known {
			return code
		}
		return codeInternal
	}
	switch err.Code() {
	case connect.CodeUnavailable:
		return codeDaemonUnavailable
	case connect.CodeDeadlineExceeded:
		return codeTimeout
	case connect.CodeCanceled:
		return codeInterrupted
	default:
		return codeInternal
	}
}

// errorSource is where a configuration error locates its problem, as loading here or in the
// daemon found it.
func errorSource(err error) *config.Source {
	var located *config.SourceError
	if errors.As(err, &located) {
		return &config.Source{Path: located.Path, Line: located.Line}
	}
	var rpc *connect.Error
	if !errors.As(err, &rpc) {
		return nil
	}
	for _, d := range rpc.Details() {
		if value, decodeErr := d.Value(); decodeErr == nil {
			if detail, ok := value.(*v1.ErrorDetail); ok && detail.Source != nil {
				return &config.Source{Path: detail.Source.Path, Line: int(detail.Source.Line)}
			}
		}
	}
	return nil
}

// printError writes err as a JSON error document; configuration errors name their location.
func printError(code string, err error) {
	type errorJSON struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Source  *config.Source `json:"source,omitempty"`
	}
	b := errorJSON{Code: code, Message: errorMessage(err)}
	if code == codeConfigInvalid {
		b.Source = errorSource(err)
	}
	// A failed write leaves only the exit status to report the error.
	_ = json.NewEncoder(scripting.stdout).Encode(struct {
		Version int       `json:"version"`
		Error   errorJSON `json:"error"`
	}{jsonVersion, b})
}

// errorMessage is err's message without the code Connect prefixes to an RPC error's: the error
// document carries the code, and people need only the message.
func errorMessage(err error) string {
	message := err.Error()
	var rpc *connect.Error
	if errors.As(err, &rpc) {
		message = strings.Replace(message, rpc.Error(), rpc.Message(), 1)
	}
	return message
}
