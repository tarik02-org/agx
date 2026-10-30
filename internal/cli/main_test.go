package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/tarik02-org/agx/internal/control"
	"github.com/tarik02-org/agx/internal/daemoncmd"
	"github.com/tarik02-org/agx/internal/platform"
)

func TestMain(m *testing.M) {
	if os.Getenv("AGX_TEST_DAEMON") == "1" && filepath.Base(os.Args[0]) == "agxd" {
		keyring.MockInit()
		os.Exit(daemoncmd.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
	}
	if output := os.Getenv("AGX_TEST_HARNESS_OUTPUT"); output != "" {
		if err := os.WriteFile(output, []byte(os.Getenv("DEFINED")), 0o600); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func stopTestDaemons(t *testing.T, cache string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(cache, "runtime", "*", "daemon.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Error(err)
			continue
		}
		var discovery control.Discovery
		if err := json.Unmarshal(data, &discovery); err != nil {
			t.Error(err)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if _, err := stopDaemon(ctx, dial(discovery), true, 0); err != nil && !daemonGone(err) {
			t.Error(err)
		}
		if err := awaitExit(ctx, platform.Paths{Runtime: filepath.Dir(path)}, 10*time.Second); err != nil {
			t.Error(err)
		}
		cancel()
	}
}
