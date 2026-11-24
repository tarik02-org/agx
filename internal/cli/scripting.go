package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/tarik02-org/agx/internal/cli/ui"
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

func printError(err error) {
	_ = json.NewEncoder(scripting.stdout).Encode(struct {
		Message string `json:"message"`
	}{errorMessage(err)})
}

// errorMessage is err's message without the code Connect prefixes to an RPC error's: the error
// document and text output need only the message.
func errorMessage(err error) string {
	message := err.Error()
	var rpc *connect.Error
	if errors.As(err, &rpc) {
		message = strings.Replace(message, rpc.Error(), rpc.Message(), 1)
	}
	return message
}
