# Isolate harnesses behind adapters; expose definitions as shims

Status: accepted.

## Adapters

Each built-in harness is a Go package implementing one interface; the rest of
agx consults a registry instead of naming harnesses.

```go
type Adapter interface {
	Name() string
	Modes() []string
	Classify(args []string) string
	Executable(configured string) (string, error)
	Directory(args []string, cwd string) (string, []string, error)
	Arguments(launch Launch) ([]string, error)
	Install(ctx context.Context, c *config.Config, cache string) error
}
```

`internal/adapters/{codex,claude,pi}` hold everything harness-specific: the mode
set and classification, passthrough commands, working-directory flags, how
skills, instructions, and MCP servers reach the harness (Codex `-c` overrides,
Claude's plugin directory and `--mcp-config`, Pi's `--skill` paths and generated
extension), and native installation (Codex's local marketplace; Claude and Pi
need none). `internal/adapters` keeps the interface, the registry, and what
adapters share: the launch plan (`Build`), compiled artifacts, native MCP server
names, TOML assignments, help/version probes, and executable lookup, whose
Windows rule (only `.exe`, since Go cannot pass arguments to a batch file
safely) applies to every harness.

`Arguments` returns only the managed prefix. `Build` locates the executable,
compiles the snapshot's artifacts into the `Launch`, and appends the
definition's groups and the caller's arguments, so argv order is the same for
every harness.

`internal/adapters/builtin` is the one explicit list of adapters; nothing
registers through `init`. The CLI builds the registry once and hands it to the
daemon, `install`, and harness commands. Configuration cannot import adapters,
which import it, so loading takes a `config.Builtins` value: the built-in names
with their modes, from the registry, and the agx command names definitions must
not shadow, from the command tree. Mode validation, `--help`'s harness list,
and the schema's mode enum all derive from it. Adding a harness means adding a
package and listing it. A declarative adapter defined in configuration was
considered and deferred until a harness needs one.

## Shims

`agx shims install` creates one executable per harness definition, so
`codex-cpa` runs `agx codex-cpa` from any shell or tool. Shims point at the agx
binary itself, which dispatches on the name it was invoked as (minus `.exe`): a
symlink on Unix, and a hard link or else a copy named `<name>.exe` on Windows,
where symlinks need Developer Mode. Linking preserves signals, exit codes, and
arguments exactly. A script (`sh`, or `.cmd` on Windows) that runs
`agx <name> <args>` is written only where neither works, and reports itself in
`agx shims list`.

A shim targets the path agx was invoked through (`argv[0]`, looked up on PATH
when bare) when that reaches the running executable and is outside
`/nix/store`, otherwise the resolved executable. Nix profiles and Homebrew
expose agx through stable links such as `~/.nix-profile/bin/agx` or
`/etc/profiles/per-user/<user>/bin/agx`, while the resolved file sits in a
versioned directory that an upgrade or garbage collection removes. A target
inside `/nix/store` gets a warning. Running `install` again after agx moves
updates the shims.

Shims go to `$XDG_DATA_HOME/agx/shims` (`~/.local/share/agx/shims`, also on macOS, where Application Support would put a space on PATH; `%LOCALAPPDATA%\agx\shims` on Windows) or `--dir`, a directory only agx writes to.
`.agx-shims.json` in that directory records each shim's file, kind, and target,
plus a content hash for hard links, copies, and scripts, so agx recognizes its
own files: `install` adds, updates, and removes shims to match the current
definitions, `remove` deletes them all, and a file agx did not create, or one
changed since, is reported and left alone. `install` and `list` warn when the
directory is not on PATH and when another executable on PATH has a shim's name.
Built-in names (`codex`, `claude`, `pi`) get no shim: shadowing the native
executables is deferred, and would need executable lookup to skip the shim
directory.
