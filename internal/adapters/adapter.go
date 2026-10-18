package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/gofrs/flock"
	"github.com/pelletier/go-toml/v2"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/securefs"
)

const GrantEnvironment = "AGX_MCP_GRANT"

type Plan struct {
	Executable  string
	Arguments   []string
	Directory   string
	Environment map[string]string
	Unset       []string
}

type Endpoint struct {
	ID  config.ID
	URL string
}
type catalog struct {
	Names []string `json:"names"`
}

const piExtension = `export default function (pi) {
  const servers = %s;
  for (const [name, config] of Object.entries(servers)) {
    config.headers.Authorization = 'Bearer ' + process.env.%s;
    pi.registerMcpServer(name, {...config, exposure: 'direct'});
  }
}
`

func FindExecutable(name, configured string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	selfInfo, err := os.Stat(self)
	if err != nil {
		return "", err
	}
	candidates := []string{}
	if configured != "" {
		candidates = append(candidates, configured)
	} else {
		extensions := []string{""}
		if runtime.GOOS == "windows" {
			extensions = strings.Split(os.Getenv("PATHEXT"), ";")
		}
		for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
			if dir == "" {
				continue
			}
			for _, ext := range extensions {
				candidates = append(candidates, filepath.Join(dir, name+ext))
			}
		}
	}
	script := false
	for _, p := range candidates {
		info, err := os.Stat(p)
		if err != nil || info.IsDir() || os.SameFile(selfInfo, info) {
			continue
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			continue
		}
		if runtime.GOOS == "windows" {
			ext := strings.ToLower(filepath.Ext(p))
			if ext == ".cmd" || ext == ".bat" {
				script = true
				continue
			}
		}
		return filepath.Abs(p)
	}
	if script {
		return "", fmt.Errorf("native %s must be an .exe executable; configure its absolute executable path", name)
	}
	return "", fmt.Errorf("native %s executable not found; configure harnesses.%s.executable", name, name)
}

// Directory resolves the launch directory from the harness's native working-directory flag
// (only Codex has one; Claude and Pi always use the process directory) and returns args with
// every such flag rewritten to the resolved absolute directory, so the harness does not apply
// a relative change a second time.
func Directory(name string, args []string, cwd string) (string, []string, error) {
	dir := cwd
	type flag struct {
		index  int
		prefix string
	}
	flags := []flag{}
	if name == "codex" {
		for i := 0; i < len(args); i++ {
			a := args[i]
			if a == "--" {
				break
			}
			switch {
			case a == "-C" || a == "--cd":
				if i+1 == len(args) {
					return "", nil, fmt.Errorf("%s requires a directory", a)
				}
				i++
				dir = args[i]
				flags = append(flags, flag{i, ""})
			case strings.HasPrefix(a, "--cd="):
				dir = strings.TrimPrefix(a, "--cd=")
				flags = append(flags, flag{i, "--cd="})
			case strings.HasPrefix(a, "-C") && len(a) > 2:
				dir = a[2:]
				flags = append(flags, flag{i, "-C"})
			}
		}
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(cwd, dir)
	}
	dir, err := config.Canonical(dir)
	if err != nil {
		return "", nil, err
	}
	rewritten := append([]string(nil), args...)
	for _, f := range flags {
		rewritten[f.index] = f.prefix + dir
	}
	return dir, rewritten, nil
}

// Mode classifies a native invocation of a built-in harness. Help/version probes and
// non-session commands are passthrough: they run without a managed launch.
func Mode(builtin string, args []string) string {
	for _, a := range args {
		if a == "--" {
			break
		}
		switch a {
		case "--help", "-h", "--version", "-V":
			return config.ModePassthrough
		case "-v":
			if builtin == "claude" || builtin == "pi" {
				return config.ModePassthrough
			}
		}
	}
	switch builtin {
	case "codex":
		return codexMode(args)
	case "claude":
		return claudeMode(args)
	case "pi":
		return piMode(args)
	}
	return ""
}

// codexValueFlags are global options taking a separate value, skipped to find the subcommand.
var codexValueFlags = []string{
	"-c", "--config", "--enable", "--disable", "--remote", "--remote-auth-token-env", "-i", "--image",
	"-m", "--model", "--local-provider", "-p", "--profile", "-s", "--sandbox", "-C", "--cd", "--add-dir",
	"-a", "--ask-for-approval",
}

func codexMode(args []string) string {
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") && args[i] != "-" {
		if args[i] == "--" {
			return config.ModeInteractive
		}
		if slices.Contains(codexValueFlags, args[i]) {
			i++
		}
		i++
	}
	if i >= len(args) {
		return config.ModeInteractive
	}
	switch args[i] {
	case "exec", "e", "review":
		return config.ModeExec
	case "resume", "fork":
		return config.ModeResume
	case "app-server":
		// Its subcommands (daemon, proxy, generate-ts, ...) manage or describe the server instead of serving.
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			return config.ModePassthrough
		}
		return config.ModeAppServer
	case "login", "logout", "plugin", "mcp", "completion", "update", "doctor", "features", "help", "debug",
		"sandbox", "apply", "a", "cloud", "exec-server", "agents", "remote-control", "app", "queue", "archive",
		"unarchive", "delete", "migrate-rollouts":
		return config.ModePassthrough
	}
	return config.ModeInteractive
}

func claudeMode(args []string) string {
	if len(args) > 0 {
		switch args[0] {
		case "auth", "plugin", "plugins", "mcp", "doctor", "install", "update", "upgrade", "setup-token", "help",
			"agents", "attach", "auto-mode", "gateway", "import", "logs", "purge", "respawn", "rm", "stop", "kill",
			"ultrareview":
			return config.ModePassthrough
		}
	}
	mode := config.ModeInteractive
	for _, a := range args {
		if a == "--" {
			break
		}
		name, _, _ := strings.Cut(a, "=")
		switch name {
		case "-p", "--print":
			// Print output and its flags apply even when a print run resumes a session.
			return config.ModePrint
		case "-r", "--resume", "-c", "--continue", "--from-pr", "--teleport":
			mode = config.ModeResume
		}
	}
	return mode
}

func piMode(args []string) string {
	if len(args) > 0 {
		switch args[0] {
		case "install", "remove", "uninstall", "update", "list", "config", "auth", "mcp":
			return config.ModePassthrough
		}
	}
	mode := config.ModeInteractive
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		name, value, inline := strings.Cut(a, "=")
		switch name {
		case "--list-models", "--export":
			return config.ModePassthrough
		case "-p", "--print":
			if mode != config.ModeRPC {
				mode = config.ModePrint
			}
		case "--mode":
			if !inline && i+1 < len(args) {
				i++
				value = args[i]
			}
			switch value {
			case "rpc":
				mode = config.ModeRPC
			case "json":
				if mode != config.ModeRPC {
					mode = config.ModePrint
				}
			}
		}
	}
	return mode
}

// Build composes argv as agx's own prefix, the definition's resolved groups, then native args.
func Build(
	s *config.Snapshot,
	h config.ResolvedHarness,
	harnessArgs []string,
	harnessEnv map[string]string,
	cache, base, grant string,
	endpoints []Endpoint,
	args []string,
) (Plan, error) {
	executable, err := FindExecutable(h.Builtin, h.Executable)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{
		Executable:  executable,
		Directory:   s.Directory,
		Environment: map[string]string{},
		Unset:       []string{"AGX_MANAGEMENT_TOKEN", "AGX_LIFECYCLE_TOKEN"},
	}
	for name, value := range harnessEnv {
		plan.Environment[name] = value
	}
	plan.Environment[GrantEnvironment] = grant
	root, err := artifacts(s, cache)
	if err != nil {
		return Plan{}, err
	}
	instructions, err := os.ReadFile(filepath.Join(root, "instructions.md"))
	if err != nil {
		return Plan{}, err
	}
	prefix := []string{}
	names := serverNames(endpoints)
	servers := map[string]any{}
	for _, endpoint := range endpoints {
		name := names[endpoint.ID]
		servers[name] = map[string]any{
			"type":    "http",
			"url":     endpoint.URL,
			"headers": map[string]string{"Authorization": "Bearer ${" + GrantEnvironment + "}"},
		}
	}
	switch h.Builtin {
	case "codex":
		// Codex has no file-based developer instructions (model_instructions_file replaces the
		// base prompt instead), so they travel in argv and replace a configured value.
		if len(instructions) > 0 {
			assignment, err := tomlAssignment("developer_instructions", string(instructions))
			if err != nil {
				return Plan{}, err
			}
			prefix = append(prefix, "-c", assignment)
		}
		registry, err := readCatalog(cache)
		if err != nil {
			return Plan{}, err
		}
		selected := map[string]bool{}
		for _, r := range s.Resources {
			if r.Kind == "skill" {
				name, err := skillName(s.Config.Skills[r.ID])
				if err != nil {
					return Plan{}, err
				}
				selected[name] = true
			}
		}
		known := map[string]bool{}
		for _, name := range registry.Names {
			known[name] = true
			prefix = append(
				prefix,
				"-c",
				`plugins.`+name+"@agx-managed"+`.enabled=`+strconv.FormatBool(selected[name]),
			)
		}
		for name := range selected {
			if !known[name] {
				return Plan{}, fmt.Errorf("selected skill plugin is not installed; run agx install")
			}
		}
		for _, endpoint := range endpoints {
			name := names[endpoint.ID]
			assignment, err := tomlAssignment(
				"mcp_servers."+name,
				map[string]any{"url": endpoint.URL, "bearer_token_env_var": GrantEnvironment},
			)
			if err != nil {
				return Plan{}, err
			}
			prefix = append(prefix, "-c", assignment)
		}
	case "claude":
		prefix = append(prefix, "--plugin-dir", filepath.Join(root, "plugin"))
		if len(instructions) > 0 {
			prefix = append(prefix, "--append-system-prompt-file", filepath.Join(root, "instructions.md"))
		}
		if len(servers) > 0 {
			b, err := json.Marshal(map[string]any{"mcpServers": servers})
			if err != nil {
				return Plan{}, err
			}
			prefix = append(prefix, "--mcp-config", string(b))
		}
	case "pi":
		for _, r := range s.Resources {
			if r.Kind == "skill" {
				prefix = append(prefix, "--skill", filepath.Join(root, "plugin", "skills", artifactName(r.ID)))
			}
		}
		if len(instructions) > 0 {
			prefix = append(prefix, "--append-system-prompt", filepath.Join(root, "instructions.md"))
		}
		if len(servers) > 0 {
			b, err := json.Marshal(servers)
			if err != nil {
				return Plan{}, err
			}
			script := fmt.Sprintf(piExtension, b, GrantEnvironment)
			// Endpoint URLs belong to a launch, so this extension is never a shared artifact.
			launchDir := filepath.Join(cache, "launches", base)
			if err := securefs.EnsureDir(launchDir); err != nil {
				return Plan{}, err
			}
			extension := filepath.Join(launchDir, "mcp-extension.mjs")
			if err := securefs.Write(extension, []byte(script), 0o600); err != nil {
				return Plan{}, err
			}
			prefix = append(prefix, "--extension", extension)
		}
	default:
		return Plan{}, fmt.Errorf("unsupported harness %q", h.Builtin)
	}
	prefix = append(prefix, harnessArgs...)
	prefix = append(prefix, args...)
	plan.Arguments = prefix
	// Shell-provided upstream secrets have reached the daemon; don't leak them into the harness.
	plan.Unset = append(plan.Unset, s.MCPEnvironment()...)
	return plan, nil
}

func tomlAssignment(key string, value any) (string, error) {
	// Marshal a one-field document to get correct TOML escaping and inline JSON-like tables.
	if str, ok := value.(string); ok {
		b, err := toml.Marshal(map[string]string{"v": str})
		if err != nil {
			return "", err
		}
		return key + "=" + strings.TrimSpace(strings.TrimPrefix(string(b), "v =")), nil
	}
	fields := value.(map[string]any)
	keys := []string{}
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, k := range keys {
		parts = append(parts, strconv.Quote(k)+" = "+strconv.Quote(fields[k].(string)))
	}
	return key + "={" + strings.Join(parts, ", ") + "}", nil
}

func artifactName(id config.ID) string { return "resource-" + config.Digest(string(id))[:16] }
func skillName(skill config.Skill) (string, error) {
	hash, err := skill.TreeHash()
	if err != nil {
		return "", err
	}
	return "agx-" + hash[:24], nil
}

func readCatalog(cache string) (catalog, error) {
	var c catalog
	b, err := os.ReadFile(filepath.Join(cache, "native", "catalog.json"))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}

func artifacts(s *config.Snapshot, cache string) (string, error) {
	parent := filepath.Join(cache, "artifacts")
	if err := securefs.EnsureDir(parent); err != nil {
		return "", err
	}
	dest := filepath.Join(parent, "v1-"+s.ID)
	if _, err := os.Stat(dest); err == nil {
		return dest, nil
	}
	tmp, err := os.MkdirTemp(parent, ".build-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	plugin := filepath.Join(tmp, "plugin")
	if err := os.MkdirAll(plugin, 0o700); err != nil {
		return "", err
	}
	if err := securefs.WriteJSON(
		filepath.Join(plugin, ".claude-plugin", "plugin.json"),
		map[string]string{"name": "agx-managed", "version": "1.0.0"},
	); err != nil {
		return "", err
	}
	var text strings.Builder
	for _, r := range s.Resources {
		switch r.Kind {
		case "skill":
			if err := s.Config.Skills[r.ID].Materialize(
				filepath.Join(plugin, "skills", artifactName(r.ID)),
			); err != nil {
				return "", err
			}
		case "instructions":
			b, err := s.Config.Instructions[r.ID].Read()
			if err != nil {
				return "", err
			}
			text.Write(b)
			text.WriteString("\n\n")
		}
	}
	if err := securefs.Write(filepath.Join(tmp, "instructions.md"), []byte(text.String()), 0o600); err != nil {
		return "", err
	}
	// Detect edits during compilation instead of caching content under the wrong snapshot ID.
	check, err := s.Config.Resolve(s.Directory, s.Harness)
	if err != nil {
		return "", err
	}
	if check.ID != s.ID {
		return "", fmt.Errorf("resources changed while preparing launch; retry")
	}
	if err := os.Rename(tmp, dest); err != nil {
		if _, statErr := os.Stat(dest); statErr != nil {
			return "", err
		}
	}
	return dest, nil
}

// InstallNative runs only from explicit installation, never from a harness launch.
func InstallNative(ctx context.Context, c *config.Config, cache string) error {
	if len(c.Skills) == 0 {
		return nil
	}
	configured := c.Harnesses["codex"].Executable
	executable, err := FindExecutable("codex", configured)
	if err != nil {
		if configured != "" {
			return err
		}
		return nil
	}
	parent := filepath.Join(cache, "native")
	if err := securefs.EnsureDir(parent); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(parent, "install.lock"))
	ok, err := lock.TryLockContext(ctx, 100000000)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("native installation is busy")
	}
	defer func() { _ = lock.Unlock() }()
	registry, err := readCatalog(cache)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, name := range registry.Names {
		known[name] = true
	}
	for _, r := range c.Skills {
		name, err := skillName(r)
		if err != nil {
			return err
		}
		dest := filepath.Join(parent, "marketplace", "plugins", name)
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			tmp, err := os.MkdirTemp(parent, ".plugin-*")
			if err != nil {
				return err
			}
			err = r.Materialize(filepath.Join(tmp, "skills", "managed"))
			if err == nil {
				err = securefs.WriteJSON(
					filepath.Join(tmp, ".codex-plugin", "plugin.json"),
					map[string]any{
						"name":        name,
						"version":     "1.0.0",
						"description": "Directory-selected skill",
						"skills":      "./skills/",
					},
				)
			}
			if err == nil {
				err = os.MkdirAll(filepath.Dir(dest), 0o700)
			}
			if err == nil {
				err = os.Rename(tmp, dest)
			}
			_ = os.RemoveAll(tmp)
			if err != nil {
				return err
			}
		}
		if !known[name] {
			registry.Names = append(registry.Names, name)
			known[name] = true
		}
	}
	sort.Strings(registry.Names)
	entries := []any{}
	for _, name := range registry.Names {
		entries = append(
			entries,
			map[string]any{
				"name":     name,
				"source":   map[string]string{"source": "local", "path": "./plugins/" + name},
				"policy":   map[string]string{"installation": "AVAILABLE", "authentication": "ON_INSTALL"},
				"category": "Productivity",
			},
		)
	}
	marketplace := filepath.Join(parent, "marketplace")
	if err := securefs.WriteJSON(
		filepath.Join(marketplace, ".agents", "plugins", "marketplace.json"),
		map[string]any{"name": "agx-managed", "plugins": entries},
	); err != nil {
		return err
	}
	invoke := func(args ...string) error {
		cmd := exec.CommandContext(ctx, executable, args...)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("native plugin installation failed: %w", err)
		}
		return nil
	}
	if err := invoke("plugin", "marketplace", "add", marketplace); err != nil {
		// A marketplace registered by an earlier installation is fine; anything else is not.
		listed, listErr := exec.CommandContext(ctx, executable, "plugin", "marketplace", "list").Output()
		real, realErr := filepath.EvalSymlinks(marketplace)
		registered := strings.Contains(string(listed), marketplace) ||
			realErr == nil && strings.Contains(string(listed), real)
		if listErr != nil || !registered {
			return err
		}
	}
	for _, name := range registry.Names {
		if err := invoke("plugin", "add", name+"@agx-managed"); err != nil {
			return err
		}
	}
	return securefs.WriteJSON(filepath.Join(parent, "catalog.json"), registry)
}

// maxServerName keeps harness tool names such as mcp__<server>__<tool> within model tool-name limits.
const maxServerName = 32

// serverNames gives each MCP server a readable native server name. The prefix keeps them apart from
// servers configured natively; a hash suffix separates IDs that sanitize to the same name.
func serverNames(endpoints []Endpoint) map[config.ID]string {
	candidates := map[config.ID]string{}
	count := map[string]int{}
	for _, endpoint := range endpoints {
		var name strings.Builder
		name.WriteString("agx-")
		for _, r := range string(endpoint.ID) {
			if r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
				name.WriteRune(r)
			} else {
				name.WriteRune('-')
			}
		}
		candidates[endpoint.ID] = name.String()
		count[name.String()]++
	}
	names := map[config.ID]string{}
	for id, name := range candidates {
		if count[name] > 1 || len(name) > maxServerName {
			suffix := "-" + config.Digest(string(id))[:8]
			name = name[:min(len(name), maxServerName-len(suffix))] + suffix
		}
		names[id] = name
	}
	return names
}
