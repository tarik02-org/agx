package cli

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/adapters/codex"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/daemon"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/shims"
)

const (
	// doctorMCPTimeout bounds each MCP server test; the servers are tested in parallel.
	doctorMCPTimeout = 20 * time.Second
	// doctorVersionTimeout bounds each harness --version probe.
	doctorVersionTimeout = 10 * time.Second
)

// minimumVersions are the oldest harness versions with every native feature agx uses
// (docs/harnesses.md, Supported versions).
var minimumVersions = map[string]string{
	"codex":  "0.160.0",
	"claude": "2.1.290",
	"pi":     "0.99.0",
}

type checkStatus string

const (
	checkPass checkStatus = "pass"
	checkWarn checkStatus = "warn"
	checkFail checkStatus = "fail"
)

// check is one row of agx doctor: what was checked, its outcome, and how to fix a warning or
// failure.
type check struct {
	Group  string      `json:"group"`
	Name   string      `json:"name"`
	Status checkStatus `json:"status"`
	Detail string      `json:"detail"`
	Hint   string      `json:"hint,omitempty"`
}

type doctorReport struct {
	Directory string   `json:"directory"`
	Root      string   `json:"root"`
	Files     []string `json:"files"`
	Checks    []check  `json:"checks"`
	Summary   struct {
		Pass int `json:"pass"`
		Warn int `json:"warn"`
		Fail int `json:"fail"`
	} `json:"summary"`
}

func doctorCommand(paths *platform.Paths, registry adapters.Registry, builtins *config.Builtins) *cobra.Command {
	all := false
	command := &cobra.Command{
		Use:     "doctor [directory]",
		Short:   "Check the configuration, harnesses, daemon, MCP servers, and secrets",
		GroupID: "runtime",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			directory := "."
			if len(args) > 0 {
				directory = args[0]
			}
			if info, err := os.Stat(directory); err != nil {
				return statError("directory", directory, err)
			} else if !info.IsDir() {
				return codedf(codeInvalidRequest, "not a directory: %s", directory)
			}
			// The report spells every path as configuration and launches do, symbolic links resolved.
			directory, err := config.Canonical(directory)
			if err != nil {
				return err
			}
			report := runDoctor(cmd.Context(), *paths, registry, *builtins, directory, all)
			if scripting.enabled {
				err = printJSON(doctorDocument(report))
			} else {
				err = printDoctor(report)
			}
			if err != nil {
				return err
			}
			if report.Summary.Fail > 0 {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	command.Flags().BoolVar(&all, "all", false, "test every MCP server, not only those selected for the directory")
	jsonFlag(command)
	return command
}

func runDoctor(
	ctx context.Context,
	paths platform.Paths,
	registry adapters.Registry,
	builtins config.Builtins,
	directory string,
	all bool,
) doctorReport {
	report := doctorReport{Directory: directory, Root: paths.Config, Files: []string{}}
	add := func(checks ...check) { report.Checks = append(report.Checks, checks...) }

	c, loaded, checks := configChecks(paths, builtins)
	add(checks...)
	if c.Root != "" {
		report.Root = c.Root
	}
	if c.Files != nil {
		report.Files = configFiles(c)
	}

	executables := map[string]string{}
	harnessRows := make([]check, len(registry.Names()))
	var wg sync.WaitGroup
	for i, name := range registry.Names() {
		adapter, _ := registry.Adapter(name)
		configured := ""
		if h, err := c.Harness(name); err == nil {
			configured = h.Executable
		}
		defined := false
		for definition := range c.Harnesses {
			if h, err := c.Harness(definition); err == nil && h.Builtin == name {
				defined = true
			}
		}
		executable, findErr := adapter.Executable(configured)
		if findErr == nil {
			executables[name] = executable
		}
		wg.Go(func() {
			output := ""
			var runErr error
			if findErr == nil {
				output, runErr = harnessVersion(ctx, executable)
			}
			shown := executable
			if findErr == nil {
				shown = shownPath(executable)
			}
			harnessRows[i] = harnessCheck(name, configured, defined, shown, findErr, output, runErr)
		})
	}
	wg.Wait()
	add(harnessRows...)

	if loaded && len(c.Skills) > 0 {
		add(codexDefinitionChecks(ctx, c, registry, paths.Cache)...)
	}

	client, daemonRow := connectDaemon(ctx, paths, c)
	add(daemonRow, stateCheck(shownPath(daemon.StatePath(paths))))
	if client != nil && loaded {
		add(mcpChecks(ctx, client, c, directory, all)...)
	}
	if client != nil {
		add(secretChecks(ctx, client, c)...)
	}
	add(sopsChecks(c, exec.LookPath)...)
	add(shimChecks(c, registry)...)

	for _, ch := range report.Checks {
		switch ch.Status {
		case checkPass:
			report.Summary.Pass++
		case checkWarn:
			report.Summary.Warn++
		case checkFail:
			report.Summary.Fail++
		}
	}
	return report
}

// configChecks loads the configuration. When it does not load, the fragments alone, or else
// only the built-ins, still let the other checks run; loaded reports a complete load.
func configChecks(paths platform.Paths, builtins config.Builtins) (*config.Config, bool, []check) {
	read, readErr := config.Read(paths.Config, builtins)
	if readErr != nil {
		return config.Bare(builtins), false, []check{{
			Group:  "config",
			Name:   "configuration",
			Status: checkFail,
			Detail: readErr.Error(),
			Hint:   "fix the reported fragment; agx config validate shows the error",
		}}
	}
	c, loadErr := config.Load(paths.Config, paths.Cache, builtins)
	return packageChecks(read, c, loadErr, filesDetail(read))
}

// filesDetail describes the files configuration was read from. A root file that does not exist
// is no file: agx reads the default configuration in its place.
func filesDetail(c *config.Config) string {
	files := configFiles(c)
	switch {
	case len(files) == len(c.Files):
		return plural(len(files), "file") + " from " + c.Root
	case len(files) == 0:
		return "no configuration file at " + c.Root + "; empty configuration"
	}
	return plural(len(files), "file") + "; no root file at " + c.Root
}

// configFiles is c.Files without a root file that does not exist.
func configFiles(c *config.Config) []string {
	if _, err := os.Stat(c.Root); !errors.Is(err, fs.ErrNotExist) {
		return c.Files
	}
	return slices.DeleteFunc(slices.Clone(c.Files), func(path string) bool { return path == c.Root })
}

// packageChecks classifies a failed full load: missing or stale packages are the installer's to
// fix, anything else is the configuration's.
func packageChecks(read, c *config.Config, loadErr error, files string) (*config.Config, bool, []check) {
	configRow := check{Group: "config", Name: "configuration", Status: checkPass, Detail: files}
	switch {
	case loadErr == nil && len(c.AllMissing()) > 0:
		// Launches go on without missing packages, so they warn instead of failing the check.
		return c, true, []check{configRow, {
			Group:  "config",
			Name:   "packages",
			Status: checkWarn,
			Detail: strings.Join(c.MissingWarnings(c.AllMissing()), "; "),
			Hint:   "run agx install",
		}}
	case loadErr == nil:
		detail := "no packages"
		if len(c.Packages) > 0 {
			detail = plural(len(c.Packages), "package") + " installed; lockfiles current"
		}
		return c, true, []check{configRow, {Group: "config", Name: "packages", Status: checkPass, Detail: detail}}
	case strings.Contains(loadErr.Error(), "run agx install") ||
		strings.Contains(loadErr.Error(), "stale dependency lock") ||
		strings.Contains(loadErr.Error(), "cache content does not match"):
		return read, false, []check{configRow, {
			Group:  "config",
			Name:   "packages",
			Status: checkFail,
			Detail: loadErr.Error(),
			Hint:   "run agx install",
		}}
	}
	configRow.Status, configRow.Detail = checkFail, loadErr.Error()
	configRow.Hint = "fix the reported definition; agx config validate shows the error"
	return read, false, []check{configRow}
}

// harnessVersion runs executable --version and returns its standard output, or its standard
// error when only that names a version: some Pi releases print the bare version there.
func harnessVersion(ctx context.Context, executable string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, doctorVersionTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, executable, "--version")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if !versionPattern.Match(stdout.Bytes()) && versionPattern.Match(stderr.Bytes()) {
		return stderr.String(), err
	}
	return stdout.String(), err
}

// versionPattern finds a version in --version output, bare (0.73.1) or within text
// (codex-cli 0.162.1, v1.2.3).
var versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+`)

// harnessCheck judges a built-in harness by its executable and --version output. A harness that
// is missing is only a warning unless an executable was configured for it; one older than the
// supported version fails when the configuration defines a harness based on it.
func harnessCheck(
	name, configured string,
	defined bool,
	executable string,
	findErr error,
	output string,
	runErr error,
) check {
	row := check{Group: "harness", Name: name}
	minimum := minimumVersions[name]
	switch {
	case findErr != nil && configured != "":
		row.Status, row.Detail = checkFail, findErr.Error()
		row.Hint = fmt.Sprintf("fix harnesses.%s.executable", name)
		return row
	case findErr != nil:
		row.Status, row.Detail = checkWarn, "not found on PATH"
		row.Hint = fmt.Sprintf("install %s %s or later to launch it", name, minimum)
		return row
	case runErr != nil:
		row.Status, row.Detail = checkWarn, fmt.Sprintf("%s --version failed: %v", executable, runErr)
		return row
	}
	version := versionPattern.FindString(output)
	if version == "" {
		row.Status, row.Detail = checkWarn, executable+": version not recognized"
		return row
	}
	row.Detail = fmt.Sprintf("%s %s (%s)", name, version, executable)
	if minimum != "" && compareVersions(version, minimum) < 0 {
		row.Status = checkWarn
		if defined {
			row.Status = checkFail
		}
		row.Detail = fmt.Sprintf("%s %s is older than the supported %s (%s)", name, version, minimum, executable)
		row.Hint = fmt.Sprintf(
			"update %s to %s or later; older versions lose skills, instructions, or MCP servers",
			name,
			minimum,
		)
		return row
	}
	row.Status = checkPass
	return row
}

// compareVersions compares dotted numeric versions.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(as), len(bs)) {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// codexDefinitionChecks checks the skill plugins of every Codex-based harness definition, each
// with the CODEX_HOME and arguments its launches from this environment use.
func codexDefinitionChecks(ctx context.Context, c *config.Config, registry adapters.Registry, cache string) []check {
	names := registry.Names()
	for name := range c.Harnesses {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	adapter, _ := registry.Adapter("codex")
	rows := []check{}
	for _, name := range names {
		h, err := c.Harness(name)
		if err != nil || h.Builtin != "codex" {
			continue
		}
		// The harness checks report an executable that is missing.
		executable, err := adapter.Executable(h.Executable)
		if err != nil {
			continue
		}
		state, err := codex.Plugins(ctx, executable, c, cache, adapters.InstallTarget(h, os.LookupEnv))
		for _, row := range codexChecks(state, err) {
			if name != "codex" {
				row.Name = name + " " + row.Name
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func codexChecks(state codex.PluginState, err error) []check {
	if err != nil {
		return []check{{Group: "codex", Name: "plugins", Status: checkFail, Detail: err.Error()}}
	}
	if state.Disabled != "" {
		return []check{{
			Group:  "codex",
			Name:   "plugins feature",
			Status: checkWarn,
			Detail: state.Disabled,
			Hint:   "Codex launches run without skills until plugins are enabled",
		}}
	}
	rows := []check{{Group: "codex", Name: "plugins feature", Status: checkPass, Detail: "enabled in " + state.Home}}
	marketplace := check{
		Group:  "codex",
		Name:   "marketplace",
		Status: checkPass,
		Detail: "registered: " + state.Marketplace,
	}
	if !state.Registered {
		marketplace.Status, marketplace.Detail = checkWarn, "agx marketplace not registered with Codex"
		marketplace.Hint = "run agx install; a Codex launch also registers it"
	}
	rows = append(rows, marketplace)
	plugins := check{
		Group:  "codex",
		Name:   "skill plugins",
		Status: checkPass,
		Detail: plural(len(state.Installed), "skill") + " installed",
	}
	if len(state.Missing) > 0 {
		plugins.Status = checkWarn
		plugins.Detail = "not installed: " + strings.Join(state.Missing, ", ")
		plugins.Hint = "run agx install; a Codex launch also installs them"
	}
	return append(rows, plugins)
}

// connectDaemon reaches a daemon of this build, starting it as other commands do, and reports
// what it found.
func connectDaemon(ctx context.Context, paths platform.Paths, c *config.Config) (*client, check) {
	row := check{Group: "daemon", Name: "daemon"}
	before, discoveryErr := daemon.ReadDiscovery(paths)
	if !autostart(c) {
		client, err := newClient(paths)
		if err != nil {
			row.Status, row.Detail = checkWarn, "not running; automatic startup is off"
			row.Hint = "run agx daemon start to test MCP servers and secrets"
			return nil, row
		}
		return daemonClientCheck(ctx, paths, client, before, discoveryErr, false)
	}
	client, err := daemonClient(ctx, paths, true)
	if err != nil {
		row.Status, row.Detail = checkFail, errorMessage(err)
		switch {
		case errors.Is(err, daemon.ErrVersionMismatch) && strings.Contains(err.Error(), "--force"):
			row.Hint = "run agx daemon stop --force, then agx doctor again"
		case errors.Is(err, daemon.ErrVersionMismatch):
			row.Hint = "run agx daemon stop, then agx doctor again"
		default:
			row.Hint = "inspect " + shownPath(filepath.Join(paths.Runtime, "daemon.log"))
		}
		return nil, row
	}
	return daemonClientCheck(ctx, paths, client, before, discoveryErr, true)
}

func daemonClientCheck(
	ctx context.Context,
	paths platform.Paths,
	c *client,
	before daemon.Discovery,
	discoveryErr error,
	started bool,
) (*client, check) {
	row := check{Group: "daemon", Name: "daemon"}
	probe, cancel := context.WithTimeout(ctx, callTimeout)
	view, err := c.runtime.GetRuntime(probe, request(&v1.GetRuntimeRequest{}, c.discovery.Token))
	cancel()
	if err != nil {
		row.Status, row.Detail = checkFail, "not answering: "+errorMessage(err)
		row.Hint = "inspect " + shownPath(filepath.Join(paths.Runtime, "daemon.log"))
		return nil, row
	}
	return c, daemonCheck(daemonFacts{
		version:        c.discovery.Version,
		config:         c.discovery.Config,
		wantConfig:     paths.Config,
		instance:       c.discovery.InstanceID,
		previous:       before.InstanceID,
		otherBuild:     errors.Is(discoveryErr, daemon.ErrVersionMismatch),
		mayHaveStarted: started,
		launches:       len(view.Msg.GetRuntime().GetLaunches()),
	})
}

// stateCheck checks the file in which the daemon keeps active launches across restarts. It holds
// only token verifiers and metadata, but no one else may read or replace it.
func stateCheck(path string) check {
	row := check{Group: "daemon", Name: "state", Status: checkPass, Detail: path}
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		row.Detail = "no saved launches"
	case err != nil:
		row.Status, row.Detail = checkFail, err.Error()
	case !info.Mode().IsRegular():
		row.Status, row.Detail = checkFail, path+" is not a regular file"
		row.Hint = "remove it; the daemon writes it again"
	// Windows protects the file with an ACL the daemon sets; mode bits say nothing there.
	case runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0:
		row.Status = checkFail
		row.Detail = fmt.Sprintf("%s is accessible to other users (mode %04o)", path, info.Mode().Perm())
		row.Hint = "run chmod 600 " + path
	default:
		switch err := daemon.CheckState(path); {
		case errors.Is(err, daemon.ErrNewerState):
			row.Status, row.Detail = checkFail, err.Error()
			row.Hint = "upgrade agx; a daemon of this agx cannot start while the file holds a newer format"
		case err != nil:
			// The daemon moves a file it cannot read aside and starts without its launches.
			row.Status, row.Detail = checkWarn, err.Error()
			row.Hint = "the next daemon moves it aside and forgets its saved launches"
		}
	}
	return row
}

type daemonFacts struct {
	version, config, wantConfig string
	instance, previous          string
	otherBuild, mayHaveStarted  bool
	launches                    int
}

func daemonCheck(f daemonFacts) check {
	row := check{Group: "daemon", Name: "daemon", Status: checkPass}
	if f.config != f.wantConfig {
		row.Status = checkFail
		row.Detail = "serves configuration " + f.config
		row.Hint = "run agx daemon stop, then agx --config " + shellQuote(f.wantConfig) + " daemon start"
		return row
	}
	switch {
	case f.mayHaveStarted && f.instance != f.previous && f.otherBuild:
		row.Detail = "replaced another build; now version " + f.version
	case f.mayHaveStarted && f.instance != f.previous:
		row.Detail = "started, version " + f.version
	default:
		launches := fmt.Sprintf("%d launches", f.launches)
		if f.launches == 1 {
			launches = "1 launch"
		}
		row.Detail = fmt.Sprintf("running, version %s, %s", f.version, launches)
	}
	return row
}

// mcpChecks tests the MCP servers selected for directory, or every server with all, in parallel.
func mcpChecks(ctx context.Context, c *client, cfg *config.Config, directory string, all bool) []check {
	ids := []config.ID{}
	if all {
		ids = slices.Sorted(maps.Keys(cfg.MCP))
	} else {
		snapshot, err := cfg.Resolve(directory, "")
		if err != nil {
			return []check{{Group: "mcp", Name: "selection", Status: checkFail, Detail: err.Error()}}
		}
		for _, r := range snapshot.Resources {
			if r.Kind == "mcp" {
				ids = append(ids, r.ID)
			}
		}
	}
	if len(ids) == 0 {
		detail := "no MCP servers selected for " + ui.ShortPath(directory, "")
		if all {
			detail = "no MCP servers configured"
		}
		return []check{{Group: "mcp", Name: "servers", Status: checkPass, Detail: detail}}
	}
	rows := make([]check, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		snapshot := &config.Snapshot{Config: cfg, Resources: []config.Resource{{Kind: "mcp", ID: id}}}
		inputs := map[string]string{}
		for _, name := range snapshot.MCPEnvironment() {
			if value, ok := os.LookupEnv(name); ok {
				inputs[name] = value
			}
		}
		wg.Go(func() {
			call, cancel := context.WithTimeout(ctx, doctorMCPTimeout)
			defer cancel()
			response, err := c.runtime.TestMcpServer(call, request(&v1.TestMcpServerRequest{
				McpId:             string(id),
				Directory:         directory,
				EnvironmentInputs: inputs,
			}, c.discovery.Token))
			var msg *v1.TestMcpServerResponse
			if err == nil {
				msg = response.Msg
			}
			rows[i] = mcpCheck(id, cfg.MCP[id], msg, err)
		})
	}
	wg.Wait()
	return rows
}

func mcpCheck(id config.ID, server config.MCPServer, response *v1.TestMcpServerResponse, err error) check {
	row := check{Group: "mcp", Name: string(id)}
	if err == nil {
		row.Status = checkPass
		row.Detail = plural(len(response.Tools), "tool")
		if name := strings.TrimSpace(response.ServerName + " " + response.ServerVersion); name != "" {
			row.Detail += " from " + name
		}
		return row
	}
	message := err.Error()
	var rpc *connect.Error
	if errors.As(err, &rpc) {
		message = rpc.Message()
	}
	switch errorCode(err) {
	case codeNeedsLogin:
		row.Status, row.Detail = checkWarn, "needs authorization: "+message
		secret := cmp.Or(server.Auth.Secret, id)
		row.Hint = "run agx secret login " + string(secret)
	case codeTimeout:
		row.Status, row.Detail = checkFail, fmt.Sprintf("no answer within %s", doctorMCPTimeout)
		row.Hint = "run agx mcp test " + string(id) + " for details"
	default:
		row.Status, row.Detail = checkFail, message
		row.Hint = "run agx mcp test " + string(id) + " for details"
	}
	return row
}

func secretChecks(ctx context.Context, c *client, cfg *config.Config) []check {
	call, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	response, err := c.secrets.GetSecretStatus(call, request(&v1.GetSecretStatusRequest{}, c.discovery.Token))
	if err != nil {
		return []check{{
			Group:  "secret",
			Name:   "secrets",
			Status: checkFail,
			Detail: errorMessage(explainSecretTimeout(err)),
		}}
	}
	statuses := response.Msg.Statuses
	slices.SortFunc(statuses, func(a, b *v1.SecretStatus) int { return strings.Compare(a.SecretId, b.SecretId) })
	rows := []check{}
	for _, s := range statuses {
		rows = append(rows, secretCheck(s, cfg.Secrets[config.ID(s.SecretId)], os.LookupEnv))
	}
	return rows
}

// secretCheck judges a secret by the daemon's view. Secrets read from the caller's environment
// exist for the daemon only at launch, so their variable is checked here instead.
func secretCheck(s *v1.SecretStatus, secret config.Secret, lookup func(string) (string, bool)) check {
	row := check{Group: "secret", Name: s.SecretId, Detail: s.Kind}
	problems := []string{}
	for _, d := range s.Diagnostics {
		problems = append(problems, d.Summary)
	}
	switch s.State {
	case v1.SecretStatus_STATE_READY:
		row.Status = checkPass
		row.Detail += ": ready"
	case v1.SecretStatus_STATE_PENDING:
		row.Status = checkWarn
		row.Detail += ": login in progress"
		row.Hint = "finish the login in the browser"
	case v1.SecretStatus_STATE_NEEDS_LOGIN:
		row.Status = checkWarn
		row.Detail += ": not stored"
		if s.Kind == "keychain" {
			row.Hint = "run agx secret set " + s.SecretId + " --stdin"
		} else {
			row.Hint = "run agx secret login " + s.SecretId
		}
	case v1.SecretStatus_STATE_UNAVAILABLE:
		row.Status = checkFail
		row.Detail += ": unavailable"
		if len(problems) > 0 {
			row.Detail += ": " + strings.Join(problems, "; ")
		}
	default:
		switch {
		case s.Kind == "environment" && secret.Variable != "":
			if value, ok := lookup(secret.Variable); ok && value != "" {
				row.Status = checkPass
				row.Detail += ": $" + secret.Variable + " is set"
			} else {
				row.Status = checkWarn
				row.Detail += ": $" + secret.Variable + " is not set here"
				row.Hint = "set " + secret.Variable + " in the shell that launches harnesses"
			}
		case s.Kind == "sops":
			row.Status = checkPass
			row.Detail += ": age key resolves at launch"
		default:
			row.Status = checkWarn
			row.Detail += ": not configured"
		}
	}
	return row
}

func sopsChecks(c *config.Config, lookPath func(string) (string, error)) []check {
	used := false
	for _, secret := range c.Secrets {
		if secret.Kind == "sops" {
			used = true
			break
		}
	}
	if !used {
		return nil
	}
	path, err := lookPath("sops")
	if err != nil {
		return []check{{
			Group:  "secret",
			Name:   "sops",
			Status: checkFail,
			Detail: "sops not found on PATH; sops secrets cannot decrypt",
			Hint:   "install sops (https://github.com/getsops/sops) on the daemon's PATH",
		}}
	}
	return []check{{Group: "secret", Name: "sops", Status: checkPass, Detail: path}}
}

// shimChecks reports the default shim directory for harness definitions: whether shims exist,
// whether the directory is on PATH, and which other executables share their names.
func shimChecks(c *config.Config, registry adapters.Registry) []check {
	names := []string{}
	for _, name := range c.HarnessNames() {
		if _, builtin := registry.Adapter(name); !builtin {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	dir, err := shims.DefaultDir()
	if err != nil {
		return []check{{Group: "shims", Name: "shims", Status: checkWarn, Detail: err.Error()}}
	}
	linked, names := linkedOnPath(names)
	if len(names) == 0 {
		return []check{{
			Group:  "shims",
			Name:   "links",
			Status: checkPass,
			Detail: plural(len(linked), "definition") + " run agx through links on PATH",
		}}
	}
	target, err := shims.Target(os.Args[0])
	if err != nil {
		return []check{{Group: "shims", Name: "shims", Status: checkWarn, Detail: err.Error()}}
	}
	entries, err := shims.List(dir, target, names)
	if err != nil {
		return []check{{Group: "shims", Name: "shims", Status: checkWarn, Detail: err.Error()}}
	}
	return shimRows(dir, entries, shims.OnPath(dir), shims.Collisions(dir, target, names))
}

// linkedOnPath splits names into those PATH already runs as agx, through links outside the shim
// directory that something else, such as a package manager, installed, and the rest.
func linkedOnPath(names []string) (linked, rest []string) {
	self, err := os.Executable()
	if err != nil {
		return nil, names
	}
	selfInfo, err := os.Stat(self)
	if err != nil {
		return nil, names
	}
	for _, name := range names {
		path, err := exec.LookPath(name)
		if err == nil && !shims.IsShim(path) {
			if info, err := os.Stat(path); err == nil && os.SameFile(selfInfo, info) {
				linked = append(linked, name)
				continue
			}
		}
		rest = append(rest, name)
	}
	return linked, rest
}

func shimRows(dir string, entries []shims.Entry, onPath bool, collisions []shims.Collision) []check {
	counts := map[shims.State][]string{}
	for _, e := range entries {
		counts[e.State] = append(counts[e.State], e.Name)
	}
	row := check{
		Group:  "shims",
		Name:   "shims",
		Status: checkPass,
		Detail: plural(len(counts[shims.Current]), "shim") + " current in " + dir,
	}
	switch {
	case len(counts[shims.Current]) == 0:
		row.Status, row.Detail = checkWarn, "none installed in "+dir
		row.Hint = "run agx shims install to run definitions by name, or use agx <definition>"
		return []check{row}
	case len(counts[shims.Missing])+len(counts[shims.Stale]) > 0:
		row.Status = checkWarn
		row.Detail = "missing or stale: " + strings.Join(
			slices.Concat(counts[shims.Missing], counts[shims.Stale]),
			", ",
		)
		row.Hint = "run agx shims install"
	case len(counts[shims.Modified])+len(counts[shims.Conflict]) > 0:
		row.Status = checkWarn
		row.Detail = "not agx's: " + strings.Join(slices.Concat(counts[shims.Modified], counts[shims.Conflict]), ", ")
		row.Hint = "remove or rename those files, then run agx shims install"
	}
	rows := []check{row}
	path := check{Group: "shims", Name: "PATH", Status: checkPass, Detail: dir + " is on PATH"}
	if !onPath {
		path.Status, path.Detail = checkWarn, dir+" is not on PATH"
		path.Hint = "add " + dir + " to PATH"
	}
	rows = append(rows, path)
	for _, collision := range collisions {
		shadow := check{Group: "shims", Name: collision.Name, Status: checkWarn}
		if collision.First {
			shadow.Detail = "runs " + collision.Path + ", not the agx shim"
			shadow.Hint = "put " + dir + " earlier on PATH"
		} else {
			shadow.Detail = "the agx shim shadows " + collision.Path
			shadow.Hint = "rename the definition if " + collision.Path + " should win"
		}
		rows = append(rows, shadow)
	}
	return rows
}

func printDoctor(report doctorReport) error {
	out := ui.Stdout
	marks := map[checkStatus]struct {
		level ui.Level
		text  string
	}{
		checkPass: {ui.OK, "ok"},
		checkWarn: {ui.Warn, "warn"},
		checkFail: {ui.Error, "fail"},
	}
	rows := [][]string{}
	hints := []string{}
	for _, ch := range report.Checks {
		mark := marks[ch.Status]
		name := ch.Name
		if ch.Group != ch.Name && ch.Group != "harness" && ch.Group != "config" {
			name = ch.Group + " " + ch.Name
		}
		rows = append(rows, []string{out.Badge(mark.level, mark.text), name, ch.Detail})
		if ch.Hint != "" {
			hints = append(hints, name+": "+ch.Hint)
		}
	}
	var b strings.Builder
	b.WriteString(out.Table([]string{"", "CHECK", "RESULT"}, rows, 2))
	if len(hints) > 0 {
		b.WriteString("\n" + out.Heading("Next steps") + "\n" + out.List(hints...))
	}
	level, text := ui.OK, "No problems found."
	switch {
	case report.Summary.Fail > 0:
		level, text = ui.Error, fmt.Sprintf(
			"%s, %s.",
			plural(report.Summary.Fail, "failure"),
			plural(report.Summary.Warn, "warning"),
		)
	case report.Summary.Warn > 0:
		level, text = ui.Warn, plural(report.Summary.Warn, "warning")+"."
	}
	b.WriteString("\n" + out.Status(level, text) + "\n")
	_, err := io.WriteString(out, b.String())
	return err
}

// shownPath spells path in a report as configuration and launches do, symbolic links resolved;
// the files themselves are still reached by the path given.
func shownPath(path string) string {
	if canonical, err := config.Canonical(path); err == nil {
		return canonical
	}
	return path
}
