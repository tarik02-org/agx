// Package appserver relays the Codex app-server protocol between a host such as T3 Code and
// one app-server process, giving every thread the resources of its own directory: each
// thread/start, thread/fork, and thread/resume gets a thread launch whose MCP servers, skill
// plugins, and instructions it adds to the request, and turn/start keeps unselected skill
// plugins disabled.
package appserver

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/adapters/relay"
)

// Relay copies newline-delimited JSON-RPC between client and server until the server's output
// ends. Threads without a directory run in the host's base directory.
func Relay(ctx context.Context, host adapters.ThreadHost, base string, client, server adapters.Stream) error {
	return relay.Run(
		ctx,
		relay.Options{ServerLabel: "codex app-server", ThreadLabel: "codex thread", Host: host, Base: base},
		protocol{},
		client,
		server,
	)
}

type protocol struct{}

func (protocol) Intercepts(method string) bool {
	switch method {
	case "thread/start", "thread/fork", "thread/resume", "turn/start", "skills/list":
		return true
	}
	return false
}

func (p protocol) Request(
	ctx context.Context,
	r *relay.Relay,
	method string,
	params *relay.Object,
) (bool, relay.Reply) {
	cwd, _ := params.String("cwd")
	switch method {
	case "thread/start":
		return start(ctx, r, params, r.Directory(cwd, ""), "")
	case "thread/fork":
		source, _ := params.String("threadId")
		return start(ctx, r, params, r.Directory(cwd, threadDirectory(ctx, r, source)), "")
	case "thread/resume":
		resumed, _ := params.String("threadId")
		// Codex keeps the developer instructions a thread started with, so only MCP servers and
		// plugins change.
		return start(ctx, r, params, r.Directory(cwd, threadDirectory(ctx, r, resumed)), resumed)
	case "turn/start":
		return turn(ctx, r, params), nil
	case "skills/list":
		return false, skills(ctx, r, *params)
	}
	return false, nil
}

func (protocol) Notification(r *relay.Relay, method string, params relay.Object) {
	switch method {
	case "thread/started":
		// Subagents run with their parent's configuration, grant included.
		if th, ok := params.Object("thread"); ok {
			id, _ := th.String("id")
			parent, _ := th.String("parentThreadId")
			r.Share(id, parent)
		}
	case "thread/closed", "thread/archived", "thread/deleted":
		id, _ := params.String("threadId")
		r.Close(id)
	}
}

// threadDirectory returns the directory of a thread, asking the server for one the relay has not
// seen; "" when unknown.
func threadDirectory(ctx context.Context, r *relay.Relay, id string) string {
	if id == "" {
		return ""
	}
	if t, ok := r.Thread(id); ok {
		return t.Directory
	}
	result, err := r.Call(ctx, "thread/read", map[string]any{"threadId": id})
	if err != nil {
		r.Host.Warn(fmt.Sprintf("cannot find the directory of codex thread %s: %v", id, err))
		return ""
	}
	if th, ok := result.Object("thread"); ok {
		if cwd, ok := th.String("cwd"); ok {
			return cwd
		}
	}
	return ""
}

// overrides merges a launch's configuration overrides into params.
func overrides(params *relay.Object, l *adapters.ThreadLaunch) {
	var ours map[string]any
	if json.Unmarshal(l.Config, &ours) != nil || len(ours) == 0 {
		return
	}
	theirs, _ := params.Object("config")
	params.Set("config", merge(theirs, ours).Marshal())
}

// start gives a new, forked, or resumed thread a fresh launch; resumed names the thread a resume
// keeps, whose instructions do not change.
func start(
	ctx context.Context,
	r *relay.Relay,
	params *relay.Object,
	directory string,
	resumed string,
) (bool, relay.Reply) {
	l := r.Prepare(ctx, directory)
	if l == nil {
		return false, nil
	}
	overrides(params, l)
	if l.Instructions != "" && resumed == "" {
		instructions := l.Instructions
		if theirs, ok := params.String("developerInstructions"); ok && theirs != "" {
			instructions += "\n\n" + theirs
		}
		params.Set("developerInstructions", relay.Encode(instructions))
	}
	return true, func(response relay.Response) relay.Answer {
		id := resumed
		if th, found := response.Result.Object("thread"); found && id == "" {
			id, _ = th.String("id")
		}
		if response.Error != nil || id == "" {
			r.Finish(l)
			return relay.Answer{}
		}
		r.Bind(id, l, directory)
		return relay.Answer{}
	}
}

// turn keeps the skill plugins the thread's directory does not select disabled. A turn moving
// to another directory switches plugins at once; its MCP servers change on the next resume.
func turn(ctx context.Context, r *relay.Relay, params *relay.Object) bool {
	id, _ := params.String("threadId")
	t, ok := r.Thread(id)
	if !ok {
		return false
	}
	disabled := t.Disabled
	if cwd, ok := params.String("cwd"); ok && cwd != "" {
		if cwd = r.Directory(cwd, ""); cwd != t.Directory {
			var err error
			if disabled, err = r.Host.DisabledPlugins(ctx, cwd); err != nil {
				r.Host.Warn(fmt.Sprintf("cannot select skills for %s: %v", cwd, err))
				return false
			}
			r.Move(id, cwd, disabled)
		}
	}
	var theirs []string
	if raw, ok := params.Get("disabledPluginIds"); ok {
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
	params.Set("disabledPluginIds", relay.Encode(ids))
	return true
}

// skills selects each listed directory's skills, which the response marks.
func skills(ctx context.Context, r *relay.Relay, params relay.Object) relay.Reply {
	var cwds []string
	if raw, ok := params.Get("cwds"); ok {
		_ = json.Unmarshal(raw, &cwds)
	}
	if len(cwds) == 0 {
		cwds = []string{r.Base}
	}
	disabled := map[string][]string{}
	for _, cwd := range cwds {
		dir := r.Directory(cwd, "")
		off, err := r.Host.DisabledPlugins(ctx, dir)
		if err != nil {
			r.Host.Warn(fmt.Sprintf("cannot select skills for %s: %v", dir, err))
			continue
		}
		disabled[cwd] = off
		disabled[dir] = off
	}
	return func(response relay.Response) relay.Answer {
		result := response.Result
		if response.Error == nil && rewriteSkills(&result, disabled) {
			return relay.Answer{Result: result}
		}
		return relay.Answer{}
	}
}

// rewriteSkills marks the skills of plugins a directory keeps disabled as disabled, since
// skills/list reports plugin skills as configured for the whole process.
func rewriteSkills(result *relay.Object, disabled map[string][]string) bool {
	entries, ok := result.Array("data")
	if !ok {
		return false
	}
	changed := false
	for i, rawEntry := range entries {
		entry, ok := relay.Parse(rawEntry)
		if !ok {
			continue
		}
		cwd, _ := entry.String("cwd")
		off, ok := disabled[cwd]
		if !ok {
			off, ok = disabled[filepath.Clean(cwd)]
		}
		if !ok || len(off) == 0 {
			continue
		}
		skills, ok := entry.Array("skills")
		if !ok {
			continue
		}
		for j, rawSkill := range skills {
			skill, ok := relay.Parse(rawSkill)
			if !ok {
				continue
			}
			if plugin, _ := skill.String("pluginId"); slices.Contains(off, plugin) {
				skill.Set("enabled", json.RawMessage("false"))
				skills[j] = skill.Marshal()
				changed = true
			}
		}
		entry.Set("skills", relay.EncodeArray(skills))
		entries[i] = entry.Marshal()
	}
	if changed {
		result.Set("data", relay.EncodeArray(entries))
	}
	return changed
}

// merge adds ours to theirs without changing a value theirs sets: shared keys whose values are
// both objects merge recursively, and theirs wins any other conflict. New keys come first, so a
// dotted key of theirs (Codex applies "mcp_servers.x" after the keys before it) still applies.
func merge(theirs relay.Object, ours map[string]any) relay.Object {
	added := relay.Object{}
	for _, key := range slices.Sorted(maps.Keys(ours)) {
		value := ours[key]
		existing, ok := theirs.Get(key)
		if !ok {
			added = append(added, relay.Member{Key: key, Value: relay.Encode(value)})
			continue
		}
		nested, isMap := value.(map[string]any)
		sub, isObject := relay.Parse(existing)
		if isMap && isObject {
			theirs.Set(key, merge(sub, nested).Marshal())
		}
	}
	return append(added, theirs...)
}
