package codex

import (
	"strings"
	"testing"
)

func TestPluginChecks(t *testing.T) {
	rows := pluginChecks(PluginState{Disabled: "Codex plugins are disabled (features.plugins = false in x)"}, nil)
	if len(rows) != 1 || rows[0].Status != "warn" || !strings.Contains(rows[0].Detail, "features.plugins = false") {
		t.Errorf("disabled = %+v", rows)
	}
	rows = pluginChecks(PluginState{Registered: true, Installed: []string{"review"}}, nil)
	if len(rows) != 3 || rows[1].Status != "pass" || rows[2].Status != "pass" {
		t.Errorf("installed = %+v", rows)
	}
	rows = pluginChecks(PluginState{Missing: []string{"review"}}, nil)
	if rows[1].Status != "warn" || rows[2].Status != "warn" || rows[2].Detail != "not installed: review" {
		t.Errorf("missing = %+v", rows)
	}
}
