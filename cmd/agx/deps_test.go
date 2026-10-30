package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestClientHasNoDatabaseOrDaemon(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "go", "list", "-deps", ".")
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	for dependency := range strings.FieldsSeq(string(output)) {
		if dependency == "database/sql" {
			t.Errorf("client includes %s", dependency)
		}
		for _, forbidden := range []string{
			"modernc.org/", "github.com/pressly/goose/",
			"github.com/tarik02-org/agx/internal/storage",
			"github.com/tarik02-org/agx/internal/daemon",
			"github.com/tarik02-org/agx/internal/configstore",
			"github.com/tarik02-org/agx/internal/secretstore",
			"github.com/tarik02-org/agx/internal/sessionstore",
		} {
			if strings.HasPrefix(dependency, forbidden) {
				t.Errorf("client includes %s", dependency)
			}
		}
	}
}
