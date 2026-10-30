package configstore_test

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/configstore"
	"github.com/tarik02-org/agx/internal/storage"
)

func TestSessionConfiguration(t *testing.T) {
	state := t.TempDir()
	root := filepath.Join(t.TempDir(), "agx.yaml")
	key := sha256.Sum256([]byte("session grant"))
	local := config.EmptyLocal()
	local.Document["mcp"] = map[string]any{
		"adhoc": map[string]any{
			"command":     []string{"tool"},
			"environment": map[string]any{"TOKEN": map[string]any{"$env": "TOKEN"}},
		},
	}
	inputs := map[config.ID]map[string]string{"adhoc": {"TOKEN": "private-session-input"}}
	revision, err := configstore.WriteSession(t.Context(), state, root, "launch", local, inputs, key)
	if err != nil || revision != 1 {
		t.Fatalf("write = %d, %v", revision, err)
	}
	restored, saved, err := configstore.ReadSession(t.Context(), state, root, "launch", key)
	if err != nil || restored.Revision != 1 || saved["adhoc"]["TOKEN"] != "private-session-input" {
		t.Fatalf("read = %v, %v, %v", restored, saved, err)
	}
	if _, _, err := configstore.ReadSession(t.Context(), state, root, "launch", [32]byte{}); err == nil {
		t.Fatal("wrong grant decrypted session inputs")
	}
	for _, name := range []string{"other", "launch"} {
		otherRoot := root
		if name == "launch" {
			otherRoot = filepath.Join(t.TempDir(), "agx.yaml")
		}
		other, values, err := configstore.ReadSession(t.Context(), state, otherRoot, name, key)
		if err != nil || other.Revision != 0 || len(values) != 0 {
			t.Fatalf("session crossed configuration or launch: %v, %v", other, err)
		}
	}
	user, err := configstore.Read(t.Context(), state, root)
	if err != nil || len(user.Document) != 0 {
		t.Fatalf("temporary data became user config: %v, %v", user, err)
	}
	data, err := os.ReadFile(storage.State(state))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("private-session-input")) {
		t.Fatal("session input persisted in plaintext")
	}
	if _, err := configstore.WriteSession(
		t.Context(),
		state,
		root,
		"launch",
		local,
		map[config.ID]map[string]string{"adhoc": {"TOKEN": "changed"}},
		key,
	); err == nil {
		t.Fatal("stale session write succeeded")
	}
	_, saved, err = configstore.ReadSession(t.Context(), state, root, "launch", key)
	if err != nil || saved["adhoc"]["TOKEN"] != "private-session-input" {
		t.Fatal("failed write changed inputs")
	}
	if err := configstore.DeleteSession(t.Context(), state, root, "launch"); err != nil {
		t.Fatal(err)
	}
	restored, saved, err = configstore.ReadSession(t.Context(), state, root, "launch", key)
	if err != nil || restored.Revision != 0 || len(saved) != 0 {
		t.Fatalf("cleanup retained state: %v, %v, %v", restored, saved, err)
	}
}

func TestSessionWithoutState(t *testing.T) {
	if _, err := configstore.WriteSession(
		t.Context(),
		"",
		"root",
		"launch",
		config.EmptyLocal(),
		nil,
		[32]byte{},
	); err == nil {
		t.Fatal("write without state succeeded")
	}
	if err := configstore.DeleteSession(t.Context(), "", "root", "launch"); err == nil {
		t.Fatal("delete without state succeeded")
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if _, _, err := configstore.ReadSession(t.Context(), missing, "root", "launch", [32]byte{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("read created missing state")
	}
}
