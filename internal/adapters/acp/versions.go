package acp

import (
	"bytes"

	"github.com/tarik02-org/agx/internal/adapters/relay"
)

// version is what differs between ACP protocol versions in the messages agx rewrites. Every
// version shares the session setup fields agx sets: cwd, mcpServers, and systemPrompt.
type version struct {
	// opens maps the methods that open a session to how they name it.
	opens map[string]opening
	// closes lists the methods after which a session no longer runs.
	closes []string
	// capabilities reads what an initialize result advertises.
	capabilities func(result relay.Object) capabilities
}

// opening is how a session setup request names its session.
type opening int

const (
	// created sessions start empty and are named by the result.
	created opening = iota
	// forked sessions copy another's history and are named by the result.
	forked
	// continued sessions are named by the request and keep their history.
	continued
)

type capabilities struct {
	// http says the agent accepts HTTP MCP servers.
	http bool
	// systemPrompt says the agent accepts a client system prompt in append mode.
	systemPrompt bool
	// antigravityPrompt says the agent takes text to append to its system prompt in the session
	// setup's _meta.antigravity.systemPrompt.append, Antigravity's extension, which it advertises
	// in its capabilities' _meta instead of the standard systemPrompt.
	antigravityPrompt bool
}

// versions are the ACP protocol versions agx rewrites, by number; others pass unchanged.
var versions = map[int]version{
	1: {
		opens: map[string]opening{
			"session/new":    created,
			"session/fork":   forked,
			"session/load":   continued,
			"session/resume": continued,
		},
		closes: []string{"session/close", "session/delete"},
		capabilities: func(result relay.Object) capabilities {
			agent, _ := result.Object("agentCapabilities")
			mcp, _ := agent.Object("mcpCapabilities")
			session, _ := agent.Object("sessionCapabilities")
			return capabilities{
				http:              isTrue(mcp, "http"),
				systemPrompt:      present(session, "systemPrompt"),
				antigravityPrompt: antigravityPrompt(agent),
			}
		},
	},
	// Version 2 is a draft: it moves capabilities under capabilities.session, makes them objects,
	// and drops session/load for session/resume.
	2: {
		opens: map[string]opening{
			"session/new":    created,
			"session/fork":   forked,
			"session/resume": continued,
		},
		closes: []string{"session/close", "session/delete"},
		capabilities: func(result relay.Object) capabilities {
			session, _ := result.Path("capabilities", "session")
			mcp, _ := session.Object("mcp")
			all, _ := result.Object("capabilities")
			return capabilities{
				http:              present(mcp, "http"),
				systemPrompt:      present(session, "systemPrompt"),
				antigravityPrompt: antigravityPrompt(all),
			}
		},
	},
}

// antigravityPrompt reads Antigravity's _meta.antigravity.systemPrompt.append capability.
func antigravityPrompt(advertised relay.Object) bool {
	prompt, _ := advertised.Path("_meta", "antigravity", "systemPrompt")
	return isTrue(prompt, "append")
}

// present reports whether key holds an object, the form of capabilities that carry options.
func present(o relay.Object, key string) bool {
	_, ok := o.Object(key)
	return ok
}

func isTrue(o relay.Object, key string) bool {
	raw, ok := o.Get(key)
	return ok && bytes.Equal(bytes.TrimSpace(raw), []byte("true"))
}
