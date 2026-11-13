package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"sync"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/adapters/relay"
)

func run(ctx context.Context, host adapters.ThreadHost, base string, client, server adapters.Stream) error {
	return relay.Run(
		ctx,
		relay.Options{ServerLabel: "ACP agent", ThreadLabel: "ACP session", Host: host, Base: base},
		&protocol{first: map[string]string{}},
		client,
		server,
	)
}

// protocol learns the negotiated version and the agent's capabilities from initialize, then
// gives every session setup request a launch for the session's directory.
type protocol struct {
	mu sync.Mutex
	// version is nil until initialize succeeds with a version agx knows.
	version *version
	caps    capabilities
	// first holds instructions for the first prompt of new sessions whose agent takes no system
	// prompt, by session ID.
	first map[string]string
	// unserved records that a warning said the agent takes no HTTP MCP servers.
	unserved bool
	// rejected records that the agent refused the system prompt it advertises, as agents that
	// implement an earlier draft of it do; sessions then take instructions as if it had none.
	rejected bool
}

func (p *protocol) Intercepts(method string) bool {
	switch method {
	case "initialize":
		return true
	case "session/prompt":
		// Prompts pass in order unless one may carry pending instructions, so a session/cancel
		// behind a prompt cannot overtake it.
		p.mu.Lock()
		defer p.mu.Unlock()
		return len(p.first) > 0
	}
	for _, v := range versions {
		if _, ok := v.opens[method]; ok || slices.Contains(v.closes, method) {
			return true
		}
	}
	return false
}

func (p *protocol) Request(
	ctx context.Context,
	r *relay.Relay,
	method string,
	params *relay.Object,
) (bool, relay.Reply) {
	if method == "initialize" {
		return false, p.initialized(r)
	}
	p.mu.Lock()
	v, caps := p.version, p.caps
	p.mu.Unlock()
	if v == nil {
		return false, nil
	}
	if method == "session/prompt" {
		return p.prompt(params), nil
	}
	if slices.Contains(v.closes, method) {
		id, _ := params.String("sessionId")
		return false, func(response relay.Response) relay.Answer {
			if response.Error == nil {
				p.forget(id)
				r.Close(id)
			}
			return relay.Answer{}
		}
	}
	opening, ok := v.opens[method]
	if !ok {
		return false, nil
	}
	return p.open(ctx, r, params, opening, caps)
}

// Notification ignores the agent's messages: sessions end by the client's requests or with the
// process.
func (*protocol) Notification(*relay.Relay, string, relay.Object) {}

// initialized handles the initialize response, which settles the protocol version.
func (p *protocol) initialized(r *relay.Relay) relay.Reply {
	return func(response relay.Response) relay.Answer {
		if response.Error != nil {
			return relay.Answer{}
		}
		result := response.Result
		raw, _ := result.Get("protocolVersion")
		number, err := strconv.Atoi(string(raw))
		v, known := versions[number]
		p.mu.Lock()
		defer p.mu.Unlock()
		if err != nil || !known {
			p.version = nil
			r.Host.Warn(
				fmt.Sprintf("ACP protocol version %s is not supported; sessions start without agx resources", raw),
			)
			return relay.Answer{}
		}
		p.version, p.caps = &v, v.capabilities(result)
		return relay.Answer{}
	}
}

// open gives a session setup request a launch for the session's directory and adds its resources
// in the forms the agent advertises.
func (p *protocol) open(
	ctx context.Context,
	r *relay.Relay,
	params *relay.Object,
	opening opening,
	caps capabilities,
) (bool, relay.Reply) {
	named, _ := params.String("sessionId")
	known := ""
	if t, ok := r.Thread(named); ok {
		known = t.Directory
	}
	cwd, _ := params.String("cwd")
	directory := r.Directory(cwd, known)
	l := r.Prepare(ctx, directory)
	if l == nil {
		return false, nil
	}
	changed := p.addServers(r, params, l, caps)
	// Without a system prompt, a new session reads the instructions in its first prompt; a forked
	// or continued one has them in its history.
	pending := ""
	if opening == created {
		pending = l.Instructions
	}
	settle := func(response relay.Response, pending string) relay.Answer {
		id := named
		if opening != continued {
			id, _ = response.Result.String("sessionId")
		}
		if response.Error != nil || id == "" {
			r.Finish(l)
			return relay.Answer{}
		}
		r.Bind(id, l, directory)
		if pending != "" {
			p.mu.Lock()
			p.first[id] = pending
			p.mu.Unlock()
		}
		return relay.Answer{}
	}
	p.mu.Lock()
	systemPrompt := caps.systemPrompt && !p.rejected
	p.mu.Unlock()
	if l.Instructions == "" || !systemPrompt {
		return changed, func(response relay.Response) relay.Answer { return settle(response, pending) }
	}
	without := slices.Clone(*params)
	addSystemPrompt(params, l.Instructions)
	return true, func(response relay.Response) relay.Answer {
		if !invalidParams(response) {
			return settle(response, "")
		}
		// An agent that refuses the system prompt it advertises may implement an earlier draft of
		// it; the session starts again without one.
		return relay.Answer{Retry: without, Next: func(response relay.Response) relay.Answer {
			if response.Error == nil {
				p.mu.Lock()
				warn := !p.rejected
				p.rejected = true
				p.mu.Unlock()
				if warn {
					r.Host.Warn("the ACP agent refused the system prompt it advertises; " +
						"instructions reach its new sessions in their first prompt")
				}
			}
			return settle(response, pending)
		}}
	}
}

// invalidParams reports whether response is a JSON-RPC invalid params error.
func invalidParams(response relay.Response) bool {
	code, _ := response.Error.Get("code")
	return string(code) == "-32602"
}

// addServers appends the launch's MCP servers to the request's, skipping names the client uses.
func (p *protocol) addServers(r *relay.Relay, params *relay.Object, l *adapters.ThreadLaunch, caps capabilities) bool {
	config, _ := relay.Parse(l.Config)
	ours, _ := config.Array("mcpServers")
	if len(ours) == 0 {
		return false
	}
	if !caps.http {
		p.mu.Lock()
		warn := !p.unserved
		p.unserved = true
		p.mu.Unlock()
		if warn {
			r.Host.Warn("the ACP agent accepts no HTTP MCP servers; its sessions start without agx MCP servers")
		}
		return false
	}
	theirs, _ := params.Array("mcpServers")
	names := []string{}
	for _, raw := range theirs {
		if server, ok := relay.Parse(raw); ok {
			name, _ := server.String("name")
			names = append(names, name)
		}
	}
	servers := slices.Clone(theirs)
	for _, raw := range ours {
		server, _ := relay.Parse(raw)
		if name, _ := server.String("name"); !slices.Contains(names, name) {
			servers = append(servers, raw)
		}
	}
	if servers == nil {
		servers = []json.RawMessage{}
	}
	params.Set("mcpServers", relay.EncodeArray(servers))
	return true
}

// addSystemPrompt appends instructions to the request's system prompt, keeping the client's
// mode and content first, or asks for an append when the client sent none.
func addSystemPrompt(params *relay.Object, instructions string) {
	prompt, ok := params.Object("systemPrompt")
	if !ok {
		prompt = relay.Object{{Key: "mode", Value: relay.Encode("append")}}
	}
	content, _ := prompt.Array("content")
	prompt.Set("content", relay.EncodeArray(append(content, text(instructions))))
	params.Set("systemPrompt", prompt.Marshal())
}

// prompt puts pending instructions before the first prompt of a new session.
func (p *protocol) prompt(params *relay.Object) bool {
	id, _ := params.String("sessionId")
	p.mu.Lock()
	instructions, ok := p.first[id]
	delete(p.first, id)
	p.mu.Unlock()
	if !ok {
		return false
	}
	content, _ := params.Array("prompt")
	params.Set("prompt", relay.EncodeArray(append([]json.RawMessage{text(instructions)}, content...)))
	return true
}

func (p *protocol) forget(id string) {
	p.mu.Lock()
	delete(p.first, id)
	p.mu.Unlock()
}

// text is a text content block.
func text(s string) json.RawMessage {
	return relay.Object{{Key: "type", Value: relay.Encode("text")}, {Key: "text", Value: relay.Encode(s)}}.Marshal()
}
