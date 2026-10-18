package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/tarik02-org/agx/internal/adapters"
)

// Sessions reads rollouts under $CODEX_HOME/sessions/YYYY/MM/DD. Names given with /rename live in
// session_index.jsonl rather than in the rollout. Subagent threads are not listed: Codex resumes
// them through their parent.
func (Adapter) Sessions(ctx context.Context, files adapters.SessionFiles) ([]adapters.Session, error) {
	home, err := adapters.StoreHome("CODEX_HOME", ".codex")
	if err != nil {
		return nil, err
	}
	paths, err := adapters.FindSessionFiles(filepath.Join(home, "sessions"), 3, "rollout-*.jsonl")
	if err != nil {
		return nil, err
	}
	sessions := adapters.ParseAll(ctx, files, paths, parseRollout)
	names, err := threadNames(filepath.Join(home, "session_index.jsonl"))
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		if name := names[sessions[i].ID]; name != "" {
			sessions[i].Title = adapters.Excerpt(name, 120)
		}
	}
	return sessions, nil
}

func (Adapter) Resume(s adapters.Session) []string { return []string{"resume", s.ID} }

// sessionIndexLimit bounds the name index, which holds one short line per renamed thread.
const sessionIndexLimit = 16 << 20

func threadNames(path string) (map[string]string, error) {
	names := map[string]string{}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return names, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(io.LimitReader(f, sessionIndexLimit))
	scanner.Buffer(make([]byte, 0, 4096), 1<<20)
	for scanner.Scan() {
		var entry struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
		}
		// Later lines rename the thread again; a damaged line names nothing.
		if json.Unmarshal(scanner.Bytes(), &entry) == nil && entry.ID != "" {
			names[entry.ID] = entry.ThreadName
		}
	}
	return names, nil
}

type rolloutLine struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type sessionMeta struct {
	ID           string          `json:"id"`
	Cwd          string          `json:"cwd"`
	Timestamp    string          `json:"timestamp"`
	ThreadSource string          `json:"thread_source"`
	Source       json.RawMessage `json:"source"`
}

// Rollouts record the user's first message three ways, depending on the Codex version: as a
// user_message event, an item_completed event, and a user message item tagged user.text. Which
// comes first varies, so the first of any wins.
type rolloutPayload struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Message string `json:"message"`
	Item    struct {
		Type    string        `json:"type"`
		Content []contentItem `json:"content"`
	} `json:"item"`
	Content  []contentItem `json:"content"`
	Metadata struct {
		Kinds []string `json:"content_item_kinds"`
	} `json:"internal_chat_message_metadata_passthrough"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

var userMarkers = [][]byte{[]byte(`"user_message"`), []byte(`"UserMessage"`), []byte(`"user.text"`)}

func parseRollout(f *os.File, info fs.FileInfo) (adapters.Session, bool, error) {
	session := adapters.Session{Updated: info.ModTime()}
	listed := true
	first := true
	var parseErr error
	err := adapters.ScanHead(f, func(line []byte) bool {
		if first {
			first = false
			var l rolloutLine
			var meta sessionMeta
			if err := json.Unmarshal(line, &l); err != nil || l.Type != "session_meta" {
				parseErr = fmt.Errorf("rollout does not start with session metadata")
				return false
			}
			if err := json.Unmarshal(l.Payload, &meta); err != nil || meta.ID == "" || meta.Cwd == "" {
				parseErr = fmt.Errorf("rollout session metadata lacks an id or directory")
				return false
			}
			if meta.ThreadSource == "subagent" || bytes.Contains(meta.Source, []byte(`"subagent"`)) {
				listed = false
				return false
			}
			session.ID = meta.ID
			session.Directory = meta.Cwd
			session.Started = adapters.Timestamp(meta.Timestamp)
			return true
		}
		if !containsAny(line, userMarkers) {
			return true
		}
		var l rolloutLine
		var p rolloutPayload
		if json.Unmarshal(line, &l) != nil || json.Unmarshal(l.Payload, &p) != nil {
			return true
		}
		text := ""
		switch {
		case l.Type == "event_msg" && p.Type == "user_message":
			text = p.Message
		case l.Type == "event_msg" && p.Type == "item_completed" && p.Item.Type == "UserMessage":
			text = firstText(p.Item.Content)
		case l.Type == "response_item" && p.Role == "user" && slices.Contains(p.Metadata.Kinds, "user.text"):
			text = firstText(p.Content)
		}
		if text == "" {
			return true
		}
		session.Message = adapters.Excerpt(text, 200)
		return false
	})
	if err != nil {
		return adapters.Session{}, false, err
	}
	if parseErr != nil {
		return adapters.Session{}, false, parseErr
	}
	if first {
		return adapters.Session{}, false, fmt.Errorf("empty rollout")
	}
	if session.Started.IsZero() {
		session.Started = session.Updated
	}
	return session, listed, nil
}

func firstText(items []contentItem) string {
	for _, item := range items {
		if (item.Type == "text" || item.Type == "input_text") && item.Text != "" {
			return item.Text
		}
	}
	return ""
}

func containsAny(line []byte, markers [][]byte) bool {
	for _, m := range markers {
		if bytes.Contains(line, m) {
			return true
		}
	}
	return false
}
