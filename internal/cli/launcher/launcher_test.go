package launcher

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/sessions"
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

// The session of a hidden harness is listed although no harness is offered for it.
func TestHarnessPickerWithoutHarnesses(t *testing.T) {
	m := newModel(Options{
		Harnesses: []Harness{{Name: "work", Builtin: "claude"}},
		Sessions: []sessions.Entry{{
			Session: adapters.Session{ID: "session", Directory: "/project"}, Builtin: "pi", Harness: "pi",
		}},
		Directory: "/project",
	})
	for i, row := range m.rows {
		if row.entry != nil {
			m.cursor = i
		}
	}
	if m.rows[m.cursor].entry == nil {
		t.Fatalf("no session row among %d", len(m.rows))
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl})
	next, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if choice := next.(model).choice; choice.Action != Resume || choice.Session.ID != "session" {
		t.Fatalf("choice = %+v", choice)
	}
}
