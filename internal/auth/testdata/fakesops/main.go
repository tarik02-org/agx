// Command fakesops stands in for sops in tests. It reads "encrypted" files as plain JSON and
// implements sops decrypt [--extract PATH] [--] FILE. A file whose content starts with FAIL
// fails like a wrong key. Each run appends its arguments to $SOPS_FAKE_LOG when set, and the
// environment variable SOPS_FAKE_FORBIDDEN names a variable that must not reach sops.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	if log := os.Getenv("SOPS_FAKE_LOG"); log != "" {
		f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintln(f, strings.Join(os.Args[1:], " "))
			_ = f.Close()
		}
	}
	if name := os.Getenv("SOPS_FAKE_FORBIDDEN"); name != "" {
		if _, leaked := os.LookupEnv(name); leaked {
			fail("environment variable " + name + " leaked into sops")
		}
	}
	args := os.Args[1:]
	if len(args) == 0 || args[0] != "decrypt" {
		fail("usage: sops decrypt [--extract PATH] FILE")
	}
	args = args[1:]
	extract := ""
	if len(args) > 1 && args[0] == "--extract" {
		extract, args = args[1], args[2:]
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) != 1 {
		fail("usage: sops decrypt [--extract PATH] FILE")
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		fail(err.Error())
	}
	if strings.HasPrefix(string(data), "FAIL") {
		fail("Error getting data key: 0 successful groups required, got 0\nGroup 0: FAILED")
	}
	if extract == "" {
		_, _ = os.Stdout.Write(data)
		return
	}
	var tree any
	if err := json.Unmarshal(data, &tree); err != nil {
		fail("Error unmarshalling input json: " + err.Error())
	}
	path := strings.TrimSuffix(strings.TrimPrefix(extract, "["), "]")
	for component := range strings.SplitSeq(path, "][") {
		name := strings.Trim(component, `"`)
		branch, ok := tree.(map[string]any)
		if !ok || branch[name] == nil {
			fail(fmt.Sprintf("error: component [%q] not found", name))
		}
		tree = branch[name]
	}
	if s, ok := tree.(string); ok {
		_, _ = os.Stdout.WriteString(s)
		return
	}
	_ = json.NewEncoder(os.Stdout).Encode(tree)
}

func fail(message string) {
	_, _ = fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
