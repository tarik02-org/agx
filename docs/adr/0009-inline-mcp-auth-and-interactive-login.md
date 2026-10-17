# Declare MCP auth inline; log in interactively

Status: accepted.

Separately declaring a secret and pointing an MCP server at it is ceremony for
the common case of one server, one account. An MCP entry's `auth` therefore takes
one of three forms, and `transport` is inferred: `url` means HTTP, `command`
means stdio. An explicit `transport` must agree, and an entry sets exactly one of
the two.

```yaml
mcp:
  aperture:
    url: https://aperture.tarik02.me/mcp   # auth omitted: OAuth on demand
  github:
    url: https://api.githubcopilot.com/mcp
    auth: {kind: oauth, scopes: [repo]}    # inline secret with options
  internal:
    url: https://internal.example/mcp
    auth: keychain                         # kind shorthand
  shared:
    url: https://shared.example/mcp
    auth: !secret team-token               # shared top-level secret
  public:
    url: https://public.example/mcp
    auth: none
```

An inline secret belongs to its MCP entry and is addressed by the entry's ID:
`agx secret login aperture`, `agx secret set internal`. The top-level `secrets:`
section remains for values several entries or `!secret` tags share; `!secret`
cannot name an inline secret. Inline and top-level secrets share one ID space for
`agx secret`, so an entry with inline or on-demand auth may not reuse a top-level
secret's ID; validation says to reference it with `!secret` or rename one. A
separate namespace (`mcp:aperture`) was rejected: it would make the common command
longer to spare a rare collision. Stored credentials of an inline secret are keyed
apart from top-level ones; on-demand OAuth and an explicit `auth: oauth` share
theirs, so spelling out the default keeps the login. A bare ID such as `auth:
team-token` reads as a kind and fails validation with a hint to use `!secret`.

Omitting `auth` on an HTTP entry means OAuth on demand. Following the MCP
authorization specification, a `401` response whose server advertises protected
resource metadata (the challenge's `resource_metadata`, or the well-known
location, validated per RFC 9728) marks the server `needs-auth` with a diagnostic
naming `agx secret login <id>` instead of failing; until then requests carry no
credential. agx answers the harness itself rather than relaying the upstream
challenge, which the harness could not satisfy. `auth: none` disables this, and a
`401` without such metadata stays an ordinary failure. Stdio entries have no
implicit auth. A login started before any `401` asks the server for its challenge
with a credential-less GET before falling back to the well-known location.

`agx secret login` blocks until the flow finishes. One server-streaming RPC,
`Login`, starts the login and reports its outcome: a first event with the
authorization URL and expiry, then success, failure, or expiry. Concurrent logins
for one secret share the attempt; the daemon cancels it, freeing its state, when
the last waiting stream ends early, so Ctrl-C or closing the terminal aborts the
login and a late callback only shows an error page. A daemon restart ends the
stream; the user logs in again. The CLI opens the authorization URL in the default
browser (`open`, `xdg-open`, or `rundll32 url.dll,FileProtocolHandler`, which
unlike `start` does not interpret URL metacharacters); `--no-browser` only prints
it. When stdout is not a terminal, stdout carries only the URL and the command
still waits.

Rejected: requiring a top-level secret for every authenticated server, the previous
shape; a detached login that returns immediately, which leaves the user polling
`secret status`.
