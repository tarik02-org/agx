// Package relay copies newline-delimited JSON-RPC between a host and one harness process that
// serves many threads, giving every thread a launch of its own. A Protocol says which client
// requests carry a thread's resources and how; the relay prepares those requests off the client
// loop, matches their responses, sends requests of its own, and ends each thread's launch with
// the thread or the process. Every other message passes byte for byte.
package relay

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

// callTimeout bounds the relay's own requests, which a client request waits for.
const callTimeout = 30 * time.Second

// Protocol is what the relay knows of one harness protocol.
type Protocol interface {
	// Intercepts reports whether the relay rewrites client requests of method. They are prepared
	// off the client loop, since they may wait for the daemon or the server, so that messages
	// behind them pass at once; a client orders nothing after a request it awaits.
	Intercepts(method string) bool
	// Request rewrites an intercepted request's params in place and reports whether it changed
	// them. A non-nil reply handles the request's response.
	Request(ctx context.Context, r *Relay, method string, params *Object) (changed bool, reply Reply)
	// Notification observes a server message that has a method: a notification, or a request
	// of the server's own, which still reaches the client unchanged.
	Notification(r *Relay, method string, params Object)
}

// Reply handles the response to a rewritten request: result is the response's result, empty
// unless an object, and ok false for an error response. It returns a result to send instead, or
// nil to forward the response unchanged.
type Reply func(result Object, ok bool) Object

// Options names what the relay serves, for its messages, and where its threads run.
type Options struct {
	// ServerLabel names the harness process in messages, such as "codex app-server".
	ServerLabel string
	// ThreadLabel names one of its threads, such as "codex thread".
	ThreadLabel string
	Host        adapters.ThreadHost
	// Base is the process's directory, where threads without one of their own run.
	Base string
}

// Thread is a thread with a launch.
type Thread struct {
	// Launch is shared with the threads it spawned.
	Launch *adapters.ThreadLaunch
	// Directory is the thread's latest working directory, and Disabled the native plugin IDs it
	// keeps disabled there.
	Directory string
	Disabled  []string
}

// Relay is one relayed process. Protocols reach it from Request and Notification.
type Relay struct {
	Options
	protocol Protocol
	// server takes whole lines under writing.
	server  io.Writer
	writing sync.Mutex
	// preparing counts requests being prepared off the client loop.
	preparing sync.WaitGroup
	// work runs host lifecycle calls in order, off the message loops.
	work chan func()

	mu      sync.Mutex
	threads map[string]*Thread
	// prepared holds launches no thread has bound yet.
	prepared map[*adapters.ThreadLaunch]bool
	// replies maps client request IDs to the handlers of their responses.
	replies map[string]Reply
	// calls maps the relay's own request IDs to their waiters.
	calls map[string]chan Object
	// dropped holds the relay's own request IDs that timed out; their late responses are dropped
	// too, since the client never sent them.
	dropped map[string]bool
	next    int
	closed  bool
}

// Run relays between client and server until the server's output ends, then ends every launch.
func Run(ctx context.Context, o Options, p Protocol, client, server adapters.Stream) error {
	r := &Relay{
		Options:  o,
		protocol: p,
		server:   server.Out,
		threads:  map[string]*Thread{},
		prepared: map[*adapters.ThreadLaunch]bool{},
		replies:  map[string]Reply{},
		calls:    map[string]chan Object{},
		dropped:  map[string]bool{},
		work:     make(chan func(), 64),
	}
	// Preparations still running when the server exits give up; their launches end below.
	prepareCtx, stopPreparing := context.WithCancel(ctx)
	defer stopPreparing()
	worked := make(chan struct{})
	go func() {
		defer close(worked)
		for f := range r.work {
			f()
		}
	}()
	go func() {
		err := r.fromClient(prepareCtx, client.In)
		if err != nil && !errors.Is(err, io.ErrClosedPipe) {
			r.Host.Warn("relaying to " + r.ServerLabel + ": " + err.Error())
		}
		// Requests the client sent before its input ended still reach the server.
		r.preparing.Wait()
		if c, ok := server.Out.(io.Closer); ok {
			_ = c.Close()
		}
	}()
	err := r.fromServer(server.In, client.Out)
	if err != nil {
		// The server must not block on output nobody reads.
		go func() { _, _ = io.Copy(io.Discard, server.In) }()
	}
	stopPreparing()
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	r.preparing.Wait()
	r.mu.Lock()
	for _, call := range r.calls {
		close(call)
	}
	r.calls = map[string]chan Object{}
	launches := []*adapters.ThreadLaunch{}
	for _, t := range r.threads {
		if !slices.Contains(launches, t.Launch) {
			launches = append(launches, t.Launch)
		}
	}
	for l := range r.prepared {
		if !slices.Contains(launches, l) {
			launches = append(launches, l)
		}
	}
	r.threads, r.prepared = map[string]*Thread{}, map[*adapters.ThreadLaunch]bool{}
	r.mu.Unlock()
	for _, l := range launches {
		r.finish(ctx, l)
	}
	close(r.work)
	<-worked
	return err
}

func (r *Relay) fromClient(ctx context.Context, in io.Reader) error {
	reader := bufio.NewReaderSize(in, 64<<10)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if !r.intercepted(line) || !r.prepareAsync(ctx, line) {
				if err := r.write(line); err != nil {
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

func (r *Relay) fromServer(in io.Reader, out io.Writer) error {
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

// intercepted reports whether line is a request the protocol rewrites.
func (r *Relay) intercepted(line []byte) bool {
	msg, ok := message(line)
	if !ok {
		return false
	}
	if _, hasID := msg.Get("id"); !hasID {
		return false
	}
	method, _ := msg.String("method")
	return r.protocol.Intercepts(method)
}

// prepareAsync prepares and sends a request off the client loop, unless the relay is closing.
func (r *Relay) prepareAsync(ctx context.Context, line []byte) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Run waits for preparations once closed, so none may start after.
	if r.closed {
		return false
	}
	r.preparing.Go(func() {
		// A failed write means the server exited, which ends the relay.
		_ = r.write(r.request(ctx, line))
	})
	return true
}

// write sends one line to the server.
func (r *Relay) write(line []byte) error {
	r.writing.Lock()
	defer r.writing.Unlock()
	_, err := r.server.Write(line)
	return err
}

// message decodes a JSON-RPC line, ignoring its line ending.
func message(line []byte) (Object, bool) {
	return Parse(bytes.TrimRight(line, "\r\n"))
}

// key identifies a request ID independently of its JSON spelling.
func key(raw json.RawMessage) string {
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return string(raw)
	}
	return b.String()
}

func (r *Relay) request(ctx context.Context, line []byte) []byte {
	msg, ok := message(line)
	if !ok {
		return line
	}
	method, _ := msg.String("method")
	id, _ := msg.Get("id")
	params, ok := msg.Object("params")
	if !ok {
		params = Object{}
	}
	changed, reply := r.protocol.Request(ctx, r, method, &params)
	if reply != nil {
		r.mu.Lock()
		r.replies[key(id)] = reply
		r.mu.Unlock()
	}
	if !changed {
		return line
	}
	msg.Set("params", params.Marshal())
	return append(msg.Marshal(), '\n')
}

func (r *Relay) response(line []byte) []byte {
	msg, ok := message(line)
	if !ok {
		return line
	}
	if method, ok := msg.String("method"); ok {
		params, _ := msg.Object("params")
		r.protocol.Notification(r, method, params)
		return line
	}
	id, ok := msg.Get("id")
	if !ok {
		return line
	}
	r.mu.Lock()
	call, own := r.calls[key(id)]
	delete(r.calls, key(id))
	late := r.dropped[key(id)]
	delete(r.dropped, key(id))
	reply := r.replies[key(id)]
	delete(r.replies, key(id))
	r.mu.Unlock()
	if own {
		call <- msg
		return nil
	}
	if late {
		return nil
	}
	if reply == nil {
		return line
	}
	_, succeeded := msg.Get("result")
	result, _ := msg.Object("result")
	if replaced := reply(result, succeeded); replaced != nil {
		msg.Set("result", replaced.Marshal())
		return append(msg.Marshal(), '\n')
	}
	return line
}

// Directory resolves a thread's directory against the base directory, defaulting to fallback
// and then to the base directory.
func (r *Relay) Directory(dir, fallback string) string {
	if dir == "" {
		dir = fallback
	}
	if dir == "" {
		return r.Base
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(r.Base, dir)
	}
	return filepath.Clean(dir)
}

// Call sends the relay's own request to the server and waits for its result.
func (r *Relay) Call(ctx context.Context, method string, params any) (Object, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, errors.New(r.ServerLabel + " has exited")
	}
	r.next++
	id := Encode("agx-relay-" + strconv.Itoa(r.next))
	wait := make(chan Object, 1)
	r.calls[key(id)] = wait
	r.mu.Unlock()
	request := Object{{"id", id}, {"method", Encode(method)}, {"params", Encode(params)}}
	if err := r.write(append(request.Marshal(), '\n')); err != nil {
		r.mu.Lock()
		delete(r.calls, key(id))
		r.mu.Unlock()
		return nil, err
	}
	timer := time.NewTimer(callTimeout)
	defer timer.Stop()
	select {
	case response, ok := <-wait:
		if !ok {
			return nil, errors.New(r.ServerLabel + " has exited")
		}
		if e, failed := response.Get("error"); failed {
			return nil, fmt.Errorf("%s failed: %s", method, e)
		}
		result, _ := response.Object("result")
		return result, nil
	case <-timer.C:
	case <-ctx.Done():
	}
	r.mu.Lock()
	if _, waiting := r.calls[key(id)]; waiting {
		delete(r.calls, key(id))
		r.dropped[key(id)] = true
	}
	r.mu.Unlock()
	return nil, fmt.Errorf("%s timed out", method)
}

// Prepare creates a thread launch for directory, or warns why the thread goes without one and
// returns nil. A launch no thread binds must be finished with Finish.
func (r *Relay) Prepare(ctx context.Context, directory string) *adapters.ThreadLaunch {
	l, err := r.Host.Prepare(ctx, directory)
	if err != nil {
		r.Host.Warn(fmt.Sprintf("%s in %s starts without agx resources: %v", r.ThreadLabel, directory, err))
		return nil
	}
	r.mu.Lock()
	r.prepared[&l] = true
	r.mu.Unlock()
	return &l
}

// Thread returns a copy of thread id.
func (r *Relay) Thread(id string) (Thread, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.threads[id]
	if !ok {
		return Thread{}, false
	}
	return *t, true
}

// Move records that thread id now works in directory, keeping disabled plugins disabled.
func (r *Relay) Move(id, directory string, disabled []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t := r.threads[id]; t != nil {
		t.Directory, t.Disabled = directory, disabled
	}
}

// Bind makes l the launch of thread id, finishing the one it replaces once no thread uses it.
func (r *Relay) Bind(id string, l *adapters.ThreadLaunch, directory string) {
	r.mu.Lock()
	previous := r.threads[id]
	r.threads[id] = &Thread{Launch: l, Directory: directory, Disabled: l.DisabledPlugins}
	delete(r.prepared, l)
	r.mu.Unlock()
	launch := *l
	r.work <- func() {
		if err := r.Host.Bind(context.Background(), launch, id); err != nil {
			r.Host.Warn(fmt.Sprintf("cannot record %s %s: %v", r.ThreadLabel, id, err))
		}
	}
	if previous != nil && previous.Launch != l {
		r.release(previous.Launch)
	}
}

// Share gives thread id, spawned by parent, its parent's launch, unless id has one.
func (r *Relay) Share(id, parent string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p := r.threads[parent]; p != nil && id != "" && r.threads[id] == nil {
		r.threads[id] = &Thread{Launch: p.Launch, Directory: p.Directory, Disabled: p.Disabled}
	}
}

// Close forgets thread id, finishing its launch once no thread uses it.
func (r *Relay) Close(id string) {
	r.mu.Lock()
	t := r.threads[id]
	delete(r.threads, id)
	r.mu.Unlock()
	if t != nil {
		r.release(t.Launch)
	}
}

// Finish ends a launch no thread bound, such as one whose request failed.
func (r *Relay) Finish(l *adapters.ThreadLaunch) {
	r.mu.Lock()
	delete(r.prepared, l)
	r.mu.Unlock()
	r.finish(context.Background(), l)
}

// release finishes l unless a thread still uses it.
func (r *Relay) release(l *adapters.ThreadLaunch) {
	r.mu.Lock()
	for _, t := range r.threads {
		if t.Launch == l {
			r.mu.Unlock()
			return
		}
	}
	r.mu.Unlock()
	r.finish(context.Background(), l)
}

func (r *Relay) finish(ctx context.Context, l *adapters.ThreadLaunch) {
	launch := *l
	r.work <- func() {
		if err := r.Host.Finish(ctx, launch); err != nil {
			r.Host.Warn(fmt.Sprintf("cannot end thread launch %s: %v", launch.ID, err))
		}
	}
}
