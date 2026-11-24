package pi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tarik02-org/agx/internal/adapters"
)

// Sessions reads <session dir>/--<escaped directory>--/<timestamp>_<id>.jsonl. The session
// directory is PI_CODING_AGENT_SESSION_DIR, else the agent settings' sessionDir, else sessions
// under PI_CODING_AGENT_DIR (~/.pi/agent).
func (Adapter) Sessions(ctx context.Context, files adapters.SessionFiles, root string) ([]adapters.Session, error) {
	// A custom session directory may hold files directly as well as per-directory folders.
	paths, err := adapters.FindSessionFiles(ctx, root, 1, "*.jsonl")
	return adapters.ParseAll(ctx, files, paths, parseSession), err
}

func (Adapter) Resume(s adapters.Session) []string { return []string{"--session", s.ID} }

func (Adapter) SessionRoot(directory string, lookup func(string) string) (string, error) {
	if dir := lookup("PI_CODING_AGENT_SESSION_DIR"); dir != "" {
		return adapters.StorePath(directory, dir)
	}
	agent, err := adapters.StoreHome("PI_CODING_AGENT_DIR", filepath.Join(".pi", "agent"), directory, lookup)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(agent, "settings.json"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	var settings struct {
		SessionDir string `json:"sessionDir"`
	}
	// Pi reports a damaged settings file itself; listing falls back to the default directory.
	if err == nil && json.Unmarshal(b, &settings) == nil && settings.SessionDir != "" {
		dir := settings.SessionDir
		if rest, ok := strings.CutPrefix(dir, "~"+string(filepath.Separator)); ok {
			home, err := adapters.StoreHome("", "", directory, lookup)
			if err != nil {
				return "", err
			}
			dir = filepath.Join(home, rest)
		}
		// A relative sessionDir follows the working directory, so it names no single store.
		if filepath.IsAbs(dir) {
			return adapters.StorePath(directory, dir)
		}
	}
	return adapters.StorePath(directory, filepath.Join(agent, "sessions"))
}

type entry struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Cwd       string `json:"cwd"`
	Timestamp string `json:"timestamp"`
	Name      string `json:"name"`
	Message   struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

var nameMarker = []byte(`"session_info"`)

func parseSession(f *os.File, info fs.FileInfo) (adapters.Session, bool, error) {
	session := adapters.Session{Updated: info.ModTime()}
	name := ""
	first := true
	var parseErr error
	err := adapters.ScanHead(f, func(line []byte) bool {
		if first {
			first = false
			var header entry
			if json.Unmarshal(line, &header) != nil || header.Type != "session" || header.ID == "" ||
				header.Cwd == "" {
				parseErr = fmt.Errorf("session file does not start with a session header")
				return false
			}
			session.ID = header.ID
			session.Directory = header.Cwd
			session.Started = adapters.Timestamp(header.Timestamp)
			return true
		}
		named := bytes.Contains(line, nameMarker)
		if session.Message != "" && !named {
			return true
		}
		var e entry
		if json.Unmarshal(line, &e) != nil {
			return true
		}
		switch {
		case e.Type == "session_info":
			name = e.Name
		case e.Type == "message" && e.Message.Role == "user" && session.Message == "":
			session.Message = adapters.Excerpt(userText(e.Message.Content), 200)
		}
		return true
	})
	if err == nil && parseErr == nil {
		err = adapters.ScanTail(f, info.Size(), func(line []byte) {
			var e entry
			if bytes.Contains(line, nameMarker) && json.Unmarshal(line, &e) == nil && e.Type == "session_info" {
				name = e.Name
			}
		})
	}
	if err != nil {
		return adapters.Session{}, false, err
	}
	if parseErr != nil {
		return adapters.Session{}, false, parseErr
	}
	if first {
		return adapters.Session{}, false, fmt.Errorf("empty session file")
	}
	session.Title = adapters.Excerpt(name, 120)
	if session.Started.IsZero() {
		session.Started = session.Updated
	}
	return session, true, nil
}

func userText(content json.RawMessage) string {
	var text string
	if json.Unmarshal(content, &text) == nil {
		return text
	}
	var items []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &items) != nil {
		return ""
	}
	for _, item := range items {
		if item.Type == "text" && item.Text != "" {
			return item.Text
		}
	}
	return ""
}
