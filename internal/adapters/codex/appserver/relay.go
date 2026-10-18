// Package appserver relays the Codex app-server protocol between a host such as T3 Code and
// one app-server process, giving every thread the resources of its own directory: each
// thread/start, thread/fork, and thread/resume gets a thread launch whose MCP servers, skill
// plugins, and instructions it adds to the request, and turn/start keeps unselected skill
// plugins disabled. Every other message passes byte for byte.
package appserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/tarik02-org/agx/internal/adapters"
)

// readTimeout bounds the relay's own thread/read, which a request waits for.
const readTimeout = 30 * time.Second

// Relay copies newline-delimited JSON-RPC between client and server until the server's output
// ends. Threads without a directory run in the host's base directory.
func Relay(ctx context.Context, host adapters.ThreadHost, base string, client, server adapters.Stream) error {
	r := &relay{
		host:    host,
		base:    base,
		server:  server.Out,
		threads: map[string]*thread{},
		pending: map[string]*pending{},
		calls:   map[string]chan object{},
		work:    make(chan func(), 64),
	}
	worked := make(chan struct{})
	go func() {
		defer close(worked)
		for f := range r.work {
			f()
		}
	}()
	go func() {
		err := r.fromClient(ctx, client.In)
		if err != nil && !errors.Is(err, io.ErrClosedPipe) {
			host.Warn("relaying to codex app-server: " + err.Error())
		}
		if c, ok := server.Out.(io.Closer); ok {
			_ = c.Close()
		}
	}()
	err := r.fromServer(server.In, client.Out)
	if err != nil {
		// The server must not block on output nobody reads.
		go func() { _, _ = io.Copy(io.Discard, server.In) }()
	}
	r.mu.Lock()
	r.closed = true
	for _, call := range r.calls {
		close(call)
	}
	r.calls = map[string]chan object{}
	launches := []*adapters.ThreadLaunch{}
	for _, t := range r.threads {
		if !slices.Contains(launches, t.launch) {
			launches = append(launches, t.launch)
		}
	}
	for _, p := range r.pending {
		if p.launch != nil && !slices.Contains(launches, p.launch) {
			launches = append(launches, p.launch)
		}
	}
	r.threads, r.pending = map[string]*thread{}, map[string]*pending{}
	r.mu.Unlock()
	for _, l := range launches {
		r.finish(ctx, l)
	}
	close(r.work)
	<-worked
	return err
}

type relay struct {
	host adapters.ThreadHost
	base string
	// server is written by the client loop only.
	server io.Writer
	// work runs host lifecycle calls in order, off the message loops.
	work chan func()

	mu      sync.Mutex
	threads map[string]*thread
	// pending maps client request IDs to what their responses settle.
	pending map[string]*pending
	// calls maps the relay's own request IDs to their waiters.
	calls  map[string]chan object
	next   int
	closed bool
}

type thread struct {
	// launch is shared with the thread's subagents.
	launch *adapters.ThreadLaunch
	// directory is the thread's latest working directory, and disabled the plugin IDs it keeps
	// disabled there.
	directory string
	disabled  []string
}

type pending struct {
	method string
	// launch is the thread launch the request prepared, nil when it prepared none.
	launch *adapters.ThreadLaunch
	// thread is the resumed thread.
	thread    string
	directory string
	// disabled holds a skills/list request's disabled plugin IDs by directory.
	disabled map[string][]string
}

func (r *relay) fromClient(ctx context.Context, in io.Reader) error {
	reader := bufio.NewReaderSize(in, 64<<10)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if _, err := r.server.Write(r.request(ctx, line)); err != nil {
				return err
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (r *relay) fromServer(in io.Reader, out io.Writer) error {
	reader := bufio.NewReaderSize(in, 64<<10)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if line = r.response(line); line != nil {
				if _, err := out.Write(line); err != nil {
					return err
				}
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// message decodes a JSON-RPC line, ignoring its line ending.
func message(line []byte) (object, bool) {
	o, ok := parseObject(bytes.TrimRight(line, "\r\n"))
	return o, ok
}

// key identifies a request ID independently of its JSON spelling.
func key(raw json.RawMessage) string {
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return string(raw)
	}
	return b.String()
}

func (r *relay) request(ctx context.Context, line []byte) []byte {
	msg, ok := message(line)
	if !ok {
		return line
	}
	method, _ := msg.string("method")
	id, hasID := msg.get("id")
	if !hasID {
		return line
	}
	params, ok := msg.object("params")
	if !ok {
		params = object{}
	}
	var p *pending
	switch method {
	case "thread/start":
		p = r.start(ctx, method, &params, r.directory(params, ""))
	case "thread/fork":
		source, _ := params.string("threadId")
		p = r.start(ctx, method, &params, r.directory(params, r.threadDirectory(ctx, source)))
	case "thread/resume":
		resumed, _ := params.string("threadId")
		p = r.resume(ctx, &params, resumed, r.directory(params, r.threadDirectory(ctx, resumed)))
	case "turn/start":
		if !r.turn(ctx, &params) {
			return line
		}
	case "skills/list":
		r.remember(id, r.skills(ctx, params))
		return line
	default:
		return line
	}
	if p != nil {
		r.remember(id, p)
	} else if method != "turn/start" {
		return line
	}
	msg.set("params", params.marshal())
	return append(msg.marshal(), '\n')
}

func (r *relay) remember(id json.RawMessage, p *pending) {
	r.mu.Lock()
	r.pending[key(id)] = p
	r.mu.Unlock()
}

// directory resolves a request's cwd against the base directory, defaulting to fallback.
func (r *relay) directory(params object, fallback string) string {
	dir, ok := params.string("cwd")
	if !ok || dir == "" {
		dir = fallback
	}
	if dir == "" {
		return r.base
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(r.base, dir)
	}
	return filepath.Clean(dir)
}

// threadDirectory returns the directory of a thread, asking the server for one the relay has not
// seen; "" when unknown.
func (r *relay) threadDirectory(ctx context.Context, id string) string {
	if id == "" {
		return ""
	}
	r.mu.Lock()
	t := r.threads[id]
	r.mu.Unlock()
	if t != nil {
		return t.directory
	}
	result, err := r.call(ctx, "thread/read", map[string]any{"threadId": id})
	if err != nil {
		r.host.Warn(fmt.Sprintf("cannot find the directory of codex thread %s: %v", id, err))
		return ""
	}
	if th, ok := result.object("thread"); ok {
		if cwd, ok := th.string("cwd"); ok {
			return cwd
		}
	}
	return ""
}

// call sends the relay's own request to the server and waits for its result.
func (r *relay) call(ctx context.Context, method string, params map[string]any) (object, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, errors.New("codex app-server has exited")
	}
	r.next++
	id := encode("agx-relay-" + strconv.Itoa(r.next))
	wait := make(chan object, 1)
	r.calls[key(id)] = wait
	r.mu.Unlock()
	request := object{{"id", id}, {"method", encode(method)}, {"params", encode(params)}}
	if _, err := r.server.Write(append(request.marshal(), '\n')); err != nil {
		return nil, err
	}
	timer := time.NewTimer(readTimeout)
	defer timer.Stop()
	select {
	case response, ok := <-wait:
		if !ok {
			return nil, errors.New("codex app-server has exited")
		}
		if e, failed := response.get("error"); failed {
			return nil, fmt.Errorf("%s failed: %s", method, e)
		}
		result, _ := response.object("result")
		return result, nil
	case <-timer.C:
	case <-ctx.Done():
	}
	r.mu.Lock()
	delete(r.calls, key(id))
	r.mu.Unlock()
	return nil, fmt.Errorf("%s timed out", method)
}

// prepare creates a thread launch, or reports why the thread goes without one.
func (r *relay) prepare(ctx context.Context, directory string) *adapters.ThreadLaunch {
	l, err := r.host.Prepare(ctx, directory)
	if err != nil {
		r.host.Warn(fmt.Sprintf("codex thread in %s starts without agx resources: %v", directory, err))
		return nil
	}
	return &l
}

// overrides merges a launch's configuration overrides into params.
func overrides(params *object, l *adapters.ThreadLaunch) {
	var ours map[string]any
	if json.Unmarshal(l.Config, &ours) != nil || len(ours) == 0 {
		return
	}
	theirs, _ := params.object("config")
	params.set("config", merge(theirs, ours).marshal())
}

func (r *relay) start(ctx context.Context, method string, params *object, directory string) *pending {
	l := r.prepare(ctx, directory)
	if l == nil {
		return nil
	}
	overrides(params, l)
	if l.Instructions != "" {
		instructions := l.Instructions
		if theirs, ok := params.string("developerInstructions"); ok && theirs != "" {
			instructions += "\n\n" + theirs
		}
		params.set("developerInstructions", encode(instructions))
	}
	return &pending{method: method, launch: l, directory: directory}
}

// resume gives a resumed thread a fresh launch. Codex keeps the developer instructions the
// thread started with, so only MCP servers and plugins change.
func (r *relay) resume(ctx context.Context, params *object, id, directory string) *pending {
	l := r.prepare(ctx, directory)
	if l == nil {
		return nil
	}
	overrides(params, l)
	return &pending{method: "thread/resume", launch: l, thread: id, directory: directory}
}

// turn keeps the skill plugins the thread's directory does not select disabled. A turn moving
// to another directory switches plugins at once; its MCP servers change on the next resume.
func (r *relay) turn(ctx context.Context, params *object) bool {
	id, _ := params.string("threadId")
	r.mu.Lock()
	t := r.threads[id]
	var disabled []string
	directory := ""
	if t != nil {
		disabled, directory = t.disabled, t.directory
	}
	r.mu.Unlock()
	if t == nil {
		return false
	}
	if cwd, ok := params.string("cwd"); ok && cwd != "" {
		if cwd = r.directory(*params, ""); cwd != directory {
			var err error
			if disabled, err = r.host.DisabledPlugins(ctx, cwd); err != nil {
				r.host.Warn(fmt.Sprintf("cannot select skills for %s: %v", cwd, err))
				return false
			}
			r.mu.Lock()
			t.directory, t.disabled = cwd, disabled
			r.mu.Unlock()
		}
	}
	var theirs []string
	if raw, ok := params.get("disabledPluginIds"); ok {
		_ = json.Unmarshal(raw, &theirs)
	}
	ids := slices.Clone(theirs)
	for _, p := range disabled {
		if !slices.Contains(ids, p) {
			ids = append(ids, p)
		}
	}
	if ids == nil {
		ids = []string{}
	}
	params.set("disabledPluginIds", encode(ids))
	return true
}

func (r *relay) skills(ctx context.Context, params object) *pending {
	var cwds []string
	if raw, ok := params.get("cwds"); ok {
		_ = json.Unmarshal(raw, &cwds)
	}
	if len(cwds) == 0 {
		cwds = []string{r.base}
	}
	p := &pending{method: "skills/list", disabled: map[string][]string{}}
	for _, cwd := range cwds {
		dir := cwd
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(r.base, dir)
		}
		disabled, err := r.host.DisabledPlugins(ctx, filepath.Clean(dir))
		if err != nil {
			r.host.Warn(fmt.Sprintf("cannot select skills for %s: %v", dir, err))
			continue
		}
		p.disabled[cwd] = disabled
		p.disabled[filepath.Clean(dir)] = disabled
	}
	return p
}

func (r *relay) response(line []byte) []byte {
	msg, ok := message(line)
	if !ok {
		return line
	}
	if method, ok := msg.string("method"); ok {
		r.notification(method, msg)
		return line
	}
	id, ok := msg.get("id")
	if !ok {
		return line
	}
	r.mu.Lock()
	call, own := r.calls[key(id)]
	delete(r.calls, key(id))
	p := r.pending[key(id)]
	delete(r.pending, key(id))
	r.mu.Unlock()
	if own {
		call <- msg
		return nil
	}
	if p == nil {
		return line
	}
	result, succeeded := msg.object("result")
	switch p.method {
	case "skills/list":
		if succeeded && r.rewriteSkills(&result, p.disabled) {
			msg.set("result", result.marshal())
			return append(msg.marshal(), '\n')
		}
	case "thread/start", "thread/fork", "thread/resume":
		if !succeeded {
			r.finish(context.Background(), p.launch)
			return line
		}
		id := p.thread
		if th, ok := result.object("thread"); ok && id == "" {
			id, _ = th.string("id")
		}
		if id == "" {
			r.finish(context.Background(), p.launch)
			return line
		}
		r.bind(id, p.launch, p.directory)
	}
	return line
}

// bind makes l the thread's launch, finishing the one it replaces once no thread uses it.
func (r *relay) bind(id string, l *adapters.ThreadLaunch, directory string) {
	r.mu.Lock()
	previous := r.threads[id]
	r.threads[id] = &thread{launch: l, directory: directory, disabled: l.DisabledPlugins}
	r.mu.Unlock()
	launch := *l
	r.work <- func() {
		if err := r.host.Bind(context.Background(), launch, id); err != nil {
			r.host.Warn(fmt.Sprintf("cannot record codex thread %s: %v", id, err))
		}
	}
	if previous != nil && previous.launch != l {
		r.release(previous.launch)
	}
}

// release finishes l unless a thread still uses it.
func (r *relay) release(l *adapters.ThreadLaunch) {
	r.mu.Lock()
	for _, t := range r.threads {
		if t.launch == l {
			r.mu.Unlock()
			return
		}
	}
	r.mu.Unlock()
	r.finish(context.Background(), l)
}

func (r *relay) finish(ctx context.Context, l *adapters.ThreadLaunch) {
	launch := *l
	r.work <- func() {
		if err := r.host.Finish(ctx, launch); err != nil {
			r.host.Warn(fmt.Sprintf("cannot end thread launch %s: %v", launch.ID, err))
		}
	}
}

func (r *relay) notification(method string, msg object) {
	params, _ := msg.object("params")
	switch method {
	case "thread/started":
		th, ok := params.object("thread")
		if !ok {
			return
		}
		id, _ := th.string("id")
		parent, _ := th.string("parentThreadId")
		r.mu.Lock()
		defer r.mu.Unlock()
		// Subagents run with their parent's configuration, grant included.
		if p := r.threads[parent]; p != nil && id != "" && r.threads[id] == nil {
			r.threads[id] = &thread{launch: p.launch, directory: p.directory, disabled: p.disabled}
		}
	case "thread/closed", "thread/archived", "thread/deleted":
		id, _ := params.string("threadId")
		r.mu.Lock()
		t := r.threads[id]
		delete(r.threads, id)
		r.mu.Unlock()
		if t != nil {
			r.release(t.launch)
		}
	}
}

// rewriteSkills marks the skills of plugins a directory keeps disabled as disabled, since
// skills/list reports plugin skills as configured for the whole process.
func (r *relay) rewriteSkills(result *object, disabled map[string][]string) bool {
	raw, ok := result.get("data")
	if !ok {
		return false
	}
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil {
		return false
	}
	changed := false
	for i, rawEntry := range entries {
		entry, ok := parseObject(rawEntry)
		if !ok {
			continue
		}
		cwd, _ := entry.string("cwd")
		off, ok := disabled[cwd]
		if !ok {
			off, ok = disabled[filepath.Clean(cwd)]
		}
		if !ok || len(off) == 0 {
			continue
		}
		var skills []json.RawMessage
		if raw, ok := entry.get("skills"); !ok || json.Unmarshal(raw, &skills) != nil {
			continue
		}
		for j, rawSkill := range skills {
			skill, ok := parseObject(rawSkill)
			if !ok {
				continue
			}
			if plugin, _ := skill.string("pluginId"); slices.Contains(off, plugin) {
				skill.set("enabled", json.RawMessage("false"))
				skills[j] = skill.marshal()
				changed = true
			}
		}
		entry.set("skills", encodeRaw(skills))
		entries[i] = entry.marshal()
	}
	if changed {
		result.set("data", encodeRaw(entries))
	}
	return changed
}

// encodeRaw joins raw values into a JSON array without re-encoding them.
func encodeRaw(values []json.RawMessage) json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('[')
	for i, v := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(v)
	}
	b.WriteByte(']')
	return b.Bytes()
}
