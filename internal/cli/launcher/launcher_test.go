package launcher

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestHarnessesAppearAfterFirstFrame(t *testing.T) {
	m := newModel(Options{LoadSessions: func(context.Context, func(SessionUpdate) error) error { return nil }})
	if view := m.View().Content; !strings.Contains(view, "Loading harnesses") ||
		!strings.Contains(view, "New session") {
		t.Fatalf("first frame = %s", view)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("enter launched before configuration arrived")
	}
	next, _ = next.Update(
		sessionResult{update: SessionUpdate{Harnesses: []Harness{{Name: "work", Builtin: "claude"}}, Loading: true}},
	)
	m = next.(model)
	if view := m.View().Content; !strings.Contains(view, "work") || strings.Contains(view, "Loading harnesses") {
		t.Fatalf("loaded frame = %s", view)
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	choice := next.(model).choice
	if cmd == nil || choice.Action != Start || choice.Harness != "work" {
		t.Fatalf("choice = %+v", choice)
	}
}
