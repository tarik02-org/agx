package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/daemon"
	"github.com/tarik02-org/agx/internal/packages"
)

// scripting is the --json decision of this invocation. run decides it from the arguments, so
// usage errors cobra reports before a command runs are JSON too, and the command's parsed flag
// confirms it. In JSON mode a command writes one document, or a login's event stream, to stdout,
// never prompts, and reports errors as documents.
var scripting struct {
	enabled bool
	stdout  io.Writer
}

// jsonFlag gives a command the --json flag; scripting carries its value.
func jsonFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("json", false, "versioned JSON output; never prompts (implies --yes)")
}

// wantsJSON finds --json among the flags of a command line that cobra parses, before any "--".
func wantsJSON(cmd *cobra.Command, args []string) bool {
	if cmd != nil && cmd.DisableFlagParsing {
		return false
	}
	enabled := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		name, value, inline := strings.Cut(arg, "=")
		if name != "--json" {
			continue
		}
		enabled = true
		if inline {
			// A malformed value is a usage error, which is then reported as JSON.
			if parsed, err := strconv.ParseBool(value); err == nil {
				enabled = parsed
			}
		}
	}
	return enabled
}

const jsonVersion = 1

func printJSON(value any) error {
	return json.NewEncoder(scripting.stdout).Encode(struct {
		Version int `json:"version"`
		Data    any `json:"data"`
	}{jsonVersion, value})
}

func printProto(value proto.Message) error {
	b, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(value)
	if err != nil {
		return err
	}
	return printJSON(json.RawMessage(b))
}

// event leads each line of an event stream, such as a login's; the event's fields follow it.
type event struct {
	Version int    `json:"version"`
	Event   string `json:"event"`
}

func newEvent(name string) event { return event{Version: jsonVersion, Event: name} }

// printEvent writes one line of an event stream.
func printEvent(value any) error {
	return json.NewEncoder(scripting.stdout).Encode(value)
}

// hint writes a note for a person; in JSON mode only a terminal on stderr gets it.
func hint(text string) {
	if scripting.enabled && !ui.Stderr.Terminal() {
		return
	}
	ui.Note(text)
}

// Error codes of JSON error documents. They are part of the output contract (docs/cli.md).
const (
	codeUsage              = "usage"
	codeConfigInvalid      = "config_invalid"
	codeNotFound           = "not_found"
	codeConflict           = "conflict"
	codeReadOnly           = "read_only"
	codeInvalidRequest     = "invalid_request"
	codeFailedPrecondition = "failed_precondition"
	codeNeedsLogin         = "needs_login"
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
	var rpc *connect.Error
	var netErr net.Error
	switch {
	case errors.As(err, &usage):
		return codeUsage
	case errors.As(err, &c):
		return c.code
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

// connectCode maps the daemon's Connect codes; docs/api.md fixes their meaning.
func connectCode(err *connect.Error) string {
	switch err.Code() {
	case connect.CodeNotFound:
		return codeNotFound
	case connect.CodeInvalidArgument:
		return codeInvalidRequest
	case connect.CodeFailedPrecondition:
		switch {
		case strings.HasPrefix(err.Message(), "daemon configuration invalid"):
			return codeConfigInvalid
		case strings.HasPrefix(err.Message(), "needs-auth"):
			return codeNeedsLogin
		}
		return codeFailedPrecondition
	case connect.CodeAlreadyExists, connect.CodeAborted:
		return codeConflict
	case connect.CodeUnavailable, connect.CodeUnauthenticated:
		return codeDaemonUnavailable
	case connect.CodeDeadlineExceeded:
		return codeTimeout
	case connect.CodeCanceled:
		return codeInterrupted
	default:
		return codeInternal
	}
}

type jsonSource struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

// sourcePattern finds the configuration location an error message leads with, as configuration
// errors do: "<path>:<line>: …" or "<path>: …", possibly after a prefix ending in ": ".
var (
	sourcePattern = regexp.MustCompile(`(?:^|: )((?:[A-Za-z]:)?[\\/][^:\n]*?)(?::(\d+))?: `)
	// YAML errors place the line later: "yaml: … line 4: …" or "… at L5.C1: …".
	yamlLine = regexp.MustCompile(`yaml: .*?\bline (\d+):|\bat L(\d+)\.C\d+`)
)

func errorSource(message string) *jsonSource {
	m := sourcePattern.FindStringSubmatch(message)
	if m == nil || !filepath.IsAbs(m[1]) {
		return nil
	}
	source := &jsonSource{Path: m[1]}
	line := m[2]
	if line == "" {
		if y := yamlLine.FindStringSubmatch(message); y != nil {
			line = y[1] + y[2]
		}
	}
	source.Line, _ = strconv.Atoi(line)
	return source
}

// printError writes err as a JSON error document; configuration errors name their location.
func printError(code string, err error) {
	type body struct {
		Code    string      `json:"code"`
		Message string      `json:"message"`
		Source  *jsonSource `json:"source,omitempty"`
	}
	b := body{Code: code, Message: err.Error()}
	// Connect prefixes its code, which the document already carries.
	var rpc *connect.Error
	if errors.As(err, &rpc) {
		b.Message = strings.Replace(b.Message, rpc.Error(), rpc.Message(), 1)
	}
	if code == codeConfigInvalid {
		b.Source = errorSource(b.Message)
	}
	// A failed write leaves only the exit status to report the error.
	_ = json.NewEncoder(scripting.stdout).Encode(struct {
		Version int  `json:"version"`
		Error   body `json:"error"`
	}{jsonVersion, b})
}
