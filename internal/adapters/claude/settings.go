package claude

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ValidateSettings rejects --settings in a definition's arguments: the definition's settings
// already take Claude Code's single --settings.
func (Adapter) ValidateSettings(_ map[string]any, args []string) error {
	for _, arg := range args {
		if arg == "--settings" || strings.HasPrefix(arg, "--settings=") {
			return errors.New("settings and --settings in args both set Claude Code's --settings; keep one")
		}
	}
	return nil
}

// SettingsArguments writes settings to a file of the launch and passes it as --settings, which
// Claude Code applies above the user's and the project's settings for that session only. A file
// keeps the command line short and out of ps.
func (Adapter) SettingsArguments(settings map[string]any, dir string) ([]string, error) {
	if dir == "" {
		return nil, nil
	}
	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return nil, err
	}
	return []string{"--settings", path}, nil
}
