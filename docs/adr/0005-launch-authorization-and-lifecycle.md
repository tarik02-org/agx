# Let the daemon issue launch grants and own upstream auth

The wrapper registers the harness and working directory with the daemon before
starting the harness. The daemon resolves the snapshot and issues a launch ID
and a scoped, expiring grant for its selected connections. The ID is not a
credential: HTTP MCP requests authenticate separately. Generated configuration
references runtime credentials; packages and lockfiles contain no secret values.

Mint grants in the daemon rather than distributing minting authority to wrappers.
The daemon owns credential storage and OAuth refresh, including serialization of
refreshes for shared credentials. Connections refer to named auth sources such
as OAuth, an OS-keychain secret, an environment variable, or a credential command.
Supply credentials to local upstream child processes when their integrations
require it; harnesses receive only Weft launch grants.

The wrapper supervises the harness, reports exit, and maintains a lease. Ended
or expired launches lose access and release their runtime resources. HTTP
disconnects alone do not end a launch. A launch grant does not change the
lifetime or scope of an upstream API key.
