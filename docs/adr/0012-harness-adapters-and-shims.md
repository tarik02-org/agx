# Isolate harnesses behind adapters; expose definitions as shims

Status: proposed.

## Adapters

Each built-in harness is a Go package implementing one interface and registering
itself; the rest of agx consults the registry instead of naming harnesses.

```go
type Adapter interface {
	Name() string
	Modes() []Mode
	Classify(args []string) Mode
	Directory(args []string, cwd string) (dir string, rewritten []string, err error)
	Build(launch Launch) (Plan, error)
	Install(native NativeContext) error
}
```

`internal/adapters/{codex,claude,pi}` hold everything harness-specific: mode
classification, passthrough commands, working-directory flags, how skills,
instructions, and MCP servers reach the harness, and native plugin installation.
Configuration validation, mode checks, CLI command registration, and the daemon
take mode sets and behaviour from the registry. Adding a harness means adding a
package. A declarative adapter defined in configuration was considered and
deferred until a harness needs one.

## Shims

`agx shims install` creates one executable per harness definition, so
`codex-cpa` runs `agx codex-cpa` from any shell or tool. Shims point at the agx
binary itself, which dispatches on the name it was invoked as: a symlink on
Unix, and a hard link or copy named `<name>.exe` on Windows, where symlinks need
Developer Mode. Linking preserves signals, exit codes, and arguments exactly. A
script (`sh`, or `.cmd` on Windows) is written only where neither works, and
reports itself in `agx shims list`.

Shims go to `~/.local/bin` (`%LOCALAPPDATA%\agx\bin` on Windows) or `--dir`.
`install` adds, updates, and removes shims to match the current definitions and
never replaces a file it did not create; `remove` deletes them all. Built-in
names (`codex`, `claude`, `pi`) get no shim: shadowing the native executables is
deferred, and would need executable lookup to skip the shim directory.
