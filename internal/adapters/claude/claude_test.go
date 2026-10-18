package claude_test

import (
	"testing"

	"github.com/tarik02-org/agx/internal/adapters/claude"
)

func TestClassify(t *testing.T) {
	for _, tt := range []struct {
		args []string
		mode string
	}{
		{nil, "interactive"},
		{[]string{"fix the tests"}, "interactive"},
		{[]string{"-p", "hi"}, "print"},
		{[]string{"--print"}, "print"},
		{[]string{"--print=true"}, "print"},
		{[]string{"--resume", "abc"}, "resume"},
		{[]string{"-r"}, "resume"},
		{[]string{"-c"}, "resume"},
		{[]string{"--continue"}, "resume"},
		{[]string{"--resume", "abc", "-p", "hi"}, "print"},
		{[]string{"auth", "login"}, "passthrough"},
		{[]string{"mcp", "list"}, "passthrough"},
		{[]string{"-v"}, "passthrough"},
		{[]string{"--version"}, "passthrough"},
		{[]string{"--", "-p"}, "interactive"},
	} {
		if got := (claude.Adapter{}).Classify(tt.args); got != tt.mode {
			t.Errorf("Classify(%q) = %s, want %s", tt.args, got, tt.mode)
		}
	}
}
