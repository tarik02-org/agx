package pi_test

import (
	"testing"

	"github.com/tarik02-org/agx/internal/adapters/pi"
)

func TestClassify(t *testing.T) {
	for _, tt := range []struct {
		args []string
		mode string
	}{
		{nil, "interactive"},
		{[]string{"fix the tests"}, "interactive"},
		{[]string{"--mode", "rpc"}, "rpc"},
		{[]string{"--mode=rpc"}, "rpc"},
		{[]string{"-p", "hi"}, "print"},
		{[]string{"--mode", "json"}, "print"},
		{[]string{"--mode", "rpc", "-p"}, "rpc"},
		{[]string{"-p", "--mode", "rpc"}, "rpc"},
		{[]string{"--session", "id"}, "resume"},
		{[]string{"--fork", "id"}, "resume"},
		{[]string{"-p", "--continue"}, "print"},
		{[]string{"--continue", "--mode", "rpc"}, "rpc"},
		{[]string{"--list-models"}, "passthrough"},
		{[]string{"install", "npm:x"}, "passthrough"},
		{[]string{"-v"}, "passthrough"},
		{[]string{"--", "-p"}, "interactive"},
	} {
		if got := (pi.Adapter{}).Classify(tt.args); got != tt.mode {
			t.Errorf("Classify(%q) = %s, want %s", tt.args, got, tt.mode)
		}
	}
}
