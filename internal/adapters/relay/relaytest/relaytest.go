// Package relaytest drives a relay in tests: the test plays both the host (client) and the
// harness process (server), and a fake host hands out numbered thread launches.
package relaytest

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tarik02-org/agx/internal/adapters"
)

// Host hands out launches L1, L2, ... whose configuration Config builds for their directory;
// directory /fail cannot be prepared.
type Host struct {
	Config       func(directory, id string) string
	Instructions func(directory string) string
	Disabled     map[string][]string

	mu       sync.Mutex
	next     int
	bound    map[string]string
	finished []string
	warnings []string
}

func (h *Host) Prepare(_ context.Context, directory string) (adapters.ThreadLaunch, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if directory == "/fail" {
		return adapters.ThreadLaunch{}, fmt.Errorf("cannot resolve")
	}
	h.next++
	id := fmt.Sprintf("L%d", h.next)
	l := adapters.ThreadLaunch{ID: id, Directory: directory, DisabledPlugins: h.Disabled[directory]}
	if h.Config != nil {
		l.Config = []byte(h.Config(directory, id))
	}
	if h.Instructions != nil {
		l.Instructions = h.Instructions(directory)
	}
	return l, nil
}

func (h *Host) DisabledPlugins(_ context.Context, directory string) ([]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Disabled[directory], nil
}

func (h *Host) Bind(_ context.Context, l adapters.ThreadLaunch, thread string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.bound == nil {
		h.bound = map[string]string{}
	}
	h.bound[thread] = l.ID
	return nil
}

func (h *Host) Finish(_ context.Context, l adapters.ThreadLaunch) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.finished = append(h.finished, l.ID)
	return nil
}

func (h *Host) Warn(message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.warnings = append(h.warnings, message)
}

// State returns the launch bound to each thread and the finished launches, in order.
func (h *Host) State() (map[string]string, []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return maps.Clone(h.bound), slices.Clone(h.finished)
}

func (h *Host) Warnings() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.warnings)
}

// Relay runs a relay between two pipes.
type Relay func(ctx context.Context, host adapters.ThreadHost, base string, client, server adapters.Stream) error

// Session is a running relay with base directory /base.
type Session struct {
	t                  *testing.T
	Host               *Host
	toRelay, toServer  *io.PipeWriter
	fromRelay, fromSrv *bufio.Reader
	done               chan error
}

func Start(t *testing.T, host *Host, run Relay) *Session {
	t.Helper()
	clientIn, toRelay := io.Pipe()
	fromRelay, clientOut := io.Pipe()
	serverIn, toServer := io.Pipe()
	fromServer, serverOut := io.Pipe()
	s := &Session{
		t:         t,
		Host:      host,
		toRelay:   toRelay,
		toServer:  toServer,
		fromRelay: bufio.NewReader(fromRelay),
		fromSrv:   bufio.NewReader(fromServer),
		done:      make(chan error, 1),
	}
	go func() {
		s.done <- run(
			context.Background(),
			host,
			"/base",
			adapters.Stream{In: clientIn, Out: clientOut},
			adapters.Stream{In: serverIn, Out: serverOut},
		)
	}()
	t.Cleanup(func() {
		_ = toRelay.Close()
		_ = toServer.Close()
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
			t.Error("relay did not end")
		}
	})
	return s
}

func readLine(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	line := make(chan string, 1)
	go func() {
		s, _ := r.ReadString('\n')
		line <- s
	}()
	select {
	case s := <-line:
		return strings.TrimSuffix(s, "\n")
	case <-time.After(5 * time.Second):
		t.Fatal("no line")
		return ""
	}
}

// Send writes a line from the host without waiting for it to arrive.
func (s *Session) Send(line string) {
	go func() { _, _ = io.WriteString(s.toRelay, line+"\n") }()
}

// Answer writes a line from the server without waiting for it to arrive.
func (s *Session) Answer(line string) {
	go func() { _, _ = io.WriteString(s.toServer, line+"\n") }()
}

// Received returns the next line the server receives.
func (s *Session) Received() string {
	s.t.Helper()
	return readLine(s.t, s.fromSrv)
}

// Client sends a line from the host and returns what the server receives.
func (s *Session) Client(line string) string {
	s.t.Helper()
	s.Send(line)
	return s.Received()
}

// Server sends a line from the server and returns what the host receives.
func (s *Session) Server(line string) string {
	s.t.Helper()
	s.Answer(line)
	return readLine(s.t, s.fromRelay)
}

func (s *Session) Expect(got, want string) {
	s.t.Helper()
	if got != want {
		s.t.Errorf("\n got %s\nwant %s", got, want)
	}
}

// Eventually waits for the host to reach a state, since lifecycle calls run off the message loop.
func (s *Session) Eventually(check func(bound map[string]string, finished []string) bool) {
	s.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if check(s.Host.State()) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	bound, finished := s.Host.State()
	s.t.Fatalf("host state: bound %v, finished %v", bound, finished)
}

// Exit ends the server's output and waits for the relay to end.
func (s *Session) Exit() {
	s.t.Helper()
	_ = s.toServer.Close()
	select {
	case err := <-s.done:
		s.done <- err
	case <-time.After(5 * time.Second):
		s.t.Fatal("relay did not end with the server")
	}
}
