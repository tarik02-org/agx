package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
)

// RelayInput copies stream-json messages from src to dst. In the first initialize control
// request it prepends prompt to a string appendSystemPrompt, which Claude Code uses in place of
// --append-system-prompt-file; a request without one keeps the file's content. Every other
// line is forwarded byte for byte, and the request keeps the fields agx does not know.
func RelayInput(dst io.Writer, src io.Reader, prompt string) error {
	r := bufio.NewReaderSize(src, 64<<10)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			merged, initialize := mergeInitialize(line, prompt)
			if _, err := dst.Write(merged); err != nil {
				return err
			}
			if initialize {
				break
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
	_, err := io.Copy(dst, r)
	return err
}

// mergeInitialize returns line with prompt merged when it is an initialize control request,
// and whether it is one.
func mergeInitialize(line []byte, prompt string) ([]byte, bool) {
	if !bytes.Contains(line, []byte(`"initialize"`)) {
		return line, false
	}
	var message map[string]json.RawMessage
	if json.Unmarshal(line, &message) != nil {
		return line, false
	}
	var kind string
	if json.Unmarshal(message["type"], &kind) != nil || kind != "control_request" {
		return line, false
	}
	var request map[string]json.RawMessage
	if json.Unmarshal(message["request"], &request) != nil {
		return line, false
	}
	var subtype string
	if json.Unmarshal(request["subtype"], &subtype) != nil || subtype != "initialize" {
		return line, false
	}
	var theirs string
	raw := bytes.TrimSpace(request["appendSystemPrompt"])
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &theirs) != nil {
		// Absent or not a string: the harness keeps the flag-given append, or rejects the request.
		return line, true
	}
	merged, err := encode(prompt + "\n\n" + theirs)
	if err != nil {
		return line, true
	}
	request["appendSystemPrompt"] = merged
	if message["request"], err = encode(request); err != nil {
		return line, true
	}
	out, err := encode(message)
	if err != nil {
		return line, true
	}
	return append(out, '\n'), true
}

// encode marshals without HTML escaping, so unchanged fields keep their characters.
func encode(v any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
