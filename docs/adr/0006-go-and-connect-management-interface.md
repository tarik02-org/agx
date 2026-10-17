# Use Go with a separate Connect management interface

Implement Weft in Go for its networking and subprocess workload and native
cross-platform executables. Use the official MCP Go SDK for the MCP runtime
and standard `net/http` for serving HTTP; Rust's additional implementation
complexity has no demonstrated benefit for this workload.

Define the management interface with Protobuf and generate Go and TypeScript
clients using Buf and Connect. Launcher registration, configuration resolution,
auth operations, lifecycle reporting, and status watches use this interface.
MCP tool traffic remains on native MCP endpoints rather than being tunneled
through custom application RPCs. Authenticate both interfaces on a loopback
listener, with management authority separate from launch grants.

Decode YAML into typed documents and implement composition with provenance
explicitly; Viper's generic configuration merging does not express these rules.
The future UI uses the same management interface as the CLI. Windows subprocess
cleanup and platform-specific command definitions remain explicit implementation
work, even with a shared Go codebase.
