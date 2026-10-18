package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tarik02-org/agx/internal/adapters"
)

// Sessions reads $CLAUDE_CONFIG_DIR/projects/<escaped directory>/<id>.jsonl. The escaped name is
// lossy, so the directory comes from the entries' cwd. Subagent transcripts live in nested
// directories or agent-*.jsonl files and are not listed.
func (Adapter) Sessions(ctx context.Context, files adapters.SessionFiles) ([]adapters.Session, error) {
	home, err := adapters.StoreHome("CLAUDE_CONFIG_DIR", ".claude")
	if err != nil {
		return nil, err
	}
	paths, err := adapters.FindSessionFiles(filepath.Join(home, "projects"), 1, "*.jsonl")
	if err != nil {
		return nil, err
	}
	kept := paths[:0]
	for _, p := range paths {
		if !strings.HasPrefix(filepath.Base(p), "agent-") {
			kept = append(kept, p)
		}
	}
	return adapters.ParseAll(ctx, files, kept, parseTranscript), nil
}

func (Adapter) Resume(s adapters.Session) []string { return []string{"--resume", s.ID} }

type entry struct {
	Type        string `json:"type"`
	Cwd         string `json:"cwd"`
	Timestamp   string `json:"timestamp"`
	IsSidechain bool   `json:"isSidechain"`
	IsMeta      bool   `json:"isMeta"`
	Message     struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	CustomTitle string `json:"customTitle"`
	AITitle     string `json:"aiTitle"`
	Summary     string `json:"summary"`
}

// titles collects the latest title of each kind; a title the user set outranks a generated one.
type titles struct{ custom, ai, summary string }

func (t *titles) add(e entry) {
	switch e.Type {
	case "custom-title":
		t.custom = e.CustomTitle
	case "ai-title":
		t.ai = e.AITitle
	case "summary":
		t.summary = e.Summary
	}
}

func (t titles) best() string {
	for _, title := range []string{t.custom, t.ai, t.summary} {
		if title != "" {
			return title
		}
	}
	return ""
}

var titleMarkers = [][]byte{[]byte(`"custom-title"`), []byte(`"ai-title"`), []byte(`"summary"`)}

func parseTranscript(f *os.File, info fs.FileInfo) (adapters.Session, bool, error) {
	session := adapters.Session{
		ID:      strings.TrimSuffix(filepath.Base(f.Name()), ".jsonl"),
		Updated: info.ModTime(),
	}
	var title titles
	valid := false
	sidechain := false
	err := adapters.ScanHead(f, func(line []byte) bool {
		complete := session.Message != "" && session.Directory != "" && !session.Started.IsZero()
		// Titles may follow anywhere, so once the rest is known only lines that can carry one are decoded.
		if complete && !containsAny(line, titleMarkers) {
			return true
		}
		var e entry
		if json.Unmarshal(line, &e) != nil {
			return true
		}
		valid = true
		title.add(e)
		if session.Directory == "" && e.Cwd != "" {
			session.Directory = e.Cwd
		}
		if session.Started.IsZero() {
			session.Started = adapters.Timestamp(e.Timestamp)
		}
		if session.Message == "" && e.Type == "user" && !e.IsMeta {
			if e.IsSidechain {
				sidechain = true
				return false
			}
			if text := userText(e.Message.Content); text != "" {
				session.Message = adapters.Excerpt(text, 200)
			}
		}
		return true
	})
	if err == nil {
		err = adapters.ScanTail(f, info.Size(), func(line []byte) {
			var e entry
			if containsAny(line, titleMarkers) && json.Unmarshal(line, &e) == nil {
				title.add(e)
			}
		})
	}
	if err != nil {
		return adapters.Session{}, false, err
	}
	if !valid {
		return adapters.Session{}, false, fmt.Errorf("transcript has no readable entries")
	}
	// Without a directory there is nothing to resume in: Claude wrote no conversation yet.
	if sidechain || session.Directory == "" {
		return adapters.Session{}, false, nil
	}
	session.Title = adapters.Excerpt(title.best(), 120)
	if session.Started.IsZero() {
		session.Started = session.Updated
	}
	return session, true, nil
}

// userText is the first text the user typed. Content starting with "<" is injected by Claude
// Code (command output, caveats, hook context) rather than typed.
func userText(content json.RawMessage) string {
	var text string
	if json.Unmarshal(content, &text) != nil {
		var items []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(content, &items) != nil {
			return ""
		}
		for _, item := range items {
			if item.Type == "text" {
				text = item.Text
				break
			}
		}
	}
	if strings.HasPrefix(strings.TrimSpace(text), "<") {
		return ""
	}
	return text
}

func containsAny(line []byte, markers [][]byte) bool {
	for _, m := range markers {
		if bytes.Contains(line, m) {
			return true
		}
	}
	return false
}
