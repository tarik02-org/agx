# Security policy

## Reporting a vulnerability

Report it privately through GitHub: open the repository's
[Security tab](https://github.com/tarik02-org/agx/security) and choose
"Report a vulnerability". Please do not open a public issue for it.

Include the agx version (`agx --version`), your operating system, and the
steps that show the problem.

## What counts

[The security model](docs/security.md) says what agx protects and what it does
not. In short, agx is not a sandbox: everything it starts runs as you. A
report is in scope when agx fails at something that page says it does, such as
keeping another local user away from your MCP servers and credentials, or
keeping secret values out of files and output.

## Supported versions

Fixes go into the latest release. Until the first release, that is `master`
and its nightly builds.
