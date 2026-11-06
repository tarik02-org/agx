# Manage agx from a launch's built-in MCP server

Status: accepted. Extends ADR 0022's on-demand tool calls and ADR 0029's local
configuration; replaces ADR 0022's user-authored discovery instructions.

A harness needs a stable way to discover agx resources, save configuration and
use an MCP server that was not selected when it started. Rewriting native harness
configuration cannot reliably refresh a running tool catalog, and advertising
every upstream tool increases startup work and context use.

Every managed launch gets a native MCP server named `agx`. The ID is reserved.
Multi-thread harnesses receive it per thread launch, under that launch's grant;
passthrough commands receive none. The built-in endpoint is separate from ordinary
upstream endpoints and is created lazily. Launching performs no upstream tool
discovery. Eight tools cover context, resource browsing, semantic configuration
edits, temporary attachment, tool search and tool calls. Search returns schemas
from selected and temporarily attached servers; callers discover other configured
servers through resource browsing and attach them explicitly. Search and calls
reuse existing endpoint clients and preserve MCP content and tool errors.

Resource browsing reaches all configured resources. Credential values are not
exposed: definitions retain injection references, literal credential fields are
redacted and secrets expose status. This remains a convenience boundary, not a
sandbox: the same user's processes already have management authority.

Persistent edits commit immediately through the existing semantic editor, with
revision checks, full validation, atomic batches and read-only YAML/package
ownership. They affect future launches. Temporary attachments are a separate
operation: a configured ID or an ad-hoc stdio/HTTP definition becomes reachable
immediately through `search_tools` and `call_tool`, without changing persistent
configuration or the harness's native catalog. Only temporary servers can be
detached. Explicit process-sharing settings retain their existing meaning.

Private session owners reuse relational configuration tables. Definitions and
per-server environment inputs commit together; injected inputs are encrypted
using the random launch grant's hash rather than the OS-keychain master key.
Recovery therefore needs the wrapper's existing grant and does not prompt for a
keychain. Explicit configuration literals have ordinary configuration storage
semantics; resolved secret values are never copied. Referenced secret definitions
and their dependencies are retained for recovery with their credential identity.
The private document provides consistent secret definitions across a launch's
attachments when persistent configuration changes.

Attachments survive daemon replacement and lease reattachment. Finish, revocation,
and durable launch completion delete private state; lease expiry retains it so
existing restart recovery continues to work. Abruptly abandoned wrappers may leave
private state. A new launch, including a resumed conversation, starts from its
configured selection. Persistence remains a daemon concern; neither the client
nor configuration/runtime models gain a database dependency. A future daemonless
runtime can provide the same tool interface with in-memory session state.

Rejected: refreshing native tool catalogs per harness, which has inconsistent
support; exposing every configured server's tools at launch; implicit permanent
configuration changes when attaching; another YAML editing or approval protocol;
storing captured environment inputs as plaintext; and persisting resolved secret
values in a session definition.
