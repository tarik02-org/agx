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
	Sessions(ctx context.Context, files SessionFiles) ([]Session, error)
	Resume(session Session) []string
}
```

What only some harnesses need is an optional interface an adapter may also
implement: `Installer` (native installation), `Forwarder` (caller variables that
locate the harness's configuration), `SettingsAdapter` (native settings), and
`Doctor` (checks `agx doctor` runs per definition). Only Codex implements
`Installer`, `Forwarder`, and `Doctor`, so other adapters carry no stubs and
`agx doctor` names no harness.

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

`Arguments` returns only the managed prefix. `Build` compiles the snapshot's
artifacts into the `Launch` and appends the definition's groups and the
caller's arguments, so argv order is the same for every harness.

The wrapper, not the daemon, locates the executable: a configured absolute
path, else the caller's PATH. The daemon's PATH is that of whichever shell
first started it, so it would ignore per-project mise or Nix shells. The
wrapper sends the path with the preparation (and again when it attaches the
launch to another daemon), and native installation run for the launch, such as
Codex's plugin commands, uses it.

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
symlink on Unix, which preserves signals, exit codes, and arguments exactly. On
Windows, where symlinks need Developer Mode, a shim is a copy named
`<name>.exe` that hands off to the agx recorded in `agx-target` beside it, so
an agx updated in place never leaves shims running the old build. Hard links
and scripts were dropped: a hard link keeps the old build after an update, and
one kind per platform needs no per-shim bookkeeping.

A shim targets the path agx was invoked through (`argv[0]`, looked up on PATH
when bare) when that reaches the running executable and is outside
`/nix/store`, otherwise the resolved executable. Nix profiles and Homebrew
expose agx through stable links such as `~/.nix-profile/bin/agx` or
`/etc/profiles/per-user/<user>/bin/agx`, while the resolved file sits in a
versioned directory that an upgrade or garbage collection removes. A target
inside `/nix/store` gets a warning. Running `install` again after agx moves
updates the shims.

Shims go to `$XDG_DATA_HOME/agx/shims` (`~/.local/share/agx/shims`, also on macOS, where Application Support would put a space on PATH; `%LOCALAPPDATA%\agx\shims` on Windows),
a directory agx owns outright: `install` makes it hold exactly the current
definitions' shims, removing anything else, and `remove` deletes it. A manifest
of agx's own files and a `--dir` option were dropped with the second kind of
shim: without them there is nothing to protect. `install` and `list` warn when the
directory is not on PATH and when another executable on PATH has a shim's name.
Built-in names (`codex`, `claude`, `pi`) get no shim: shadowing the native
executables is deferred. Native executable lookup still skips agx itself and the
shim directory, so a link or shim
named after a native harness cannot be taken for it. As a last guard, the
wrapper sets `AGX_WRAPPER_PID` for the harness it starts; agx started directly
by that wrapper, through a link lookup could not recognize, refuses with an
error instead of wrapping itself again.
