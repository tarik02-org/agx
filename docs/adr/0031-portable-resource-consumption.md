# Consume configuration independently of process launch and storage

Status: accepted. Revises the adapter contract in ADR 0012.

An integration running inside a harness needs configuration even when agx did not
start its process, and the consumer may use a different filesystem. AGX resolves
configuration and supplies portable resource content and MCP authorization. The
integration registers content directly or deploys files beside the tools that use
them. Package acquisition and caching remain separate from harness deployment.

`ResourceService` resolves a scope directory and prepares a resource lease without
an executable, native arguments, installation, session files or a spawn plan.
Skills carry relative file paths, bytes, executable flags and a digest of their
content tree. Instructions remain text. This representation permits a future
content-addressed store without making its on-disk layout part of the interface;
this change does not implement that store.

The scope directory belongs to the configuration's namespace. A consumer maps its
workspace into that namespace, without requiring the path to exist on the daemon
host. A separate, explicit daemon-side directory is required when MCP processes
inherit a working directory or share per directory/repository. Native process
launches retain their existing local directory validation and MCP behavior.

Consumer identity, process launching and local session-file access are separate
adapter capabilities. CLI adapters opt into filesystem deployment; direct-content
consumers do not create placeholder files. The harness owns its conversations and
native session store. AGX owns lease state and native-launch associations, and may
read session metadata through an optional integration.

Resource leases reuse renewal, revocation and restart attachment. The integration
retains the grant, lifecycle credential, snapshot and environment inputs, plus its
daemon-side MCP directory, and releases the lease when it stops consuming those
resources. No process PID or session ID is necessary. Remote daemon transport and
the OpenCode plugin itself are separate work; daemon access remains loopback.
