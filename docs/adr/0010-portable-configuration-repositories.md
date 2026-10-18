# Keep configuration portable: sops secrets

Status: accepted.

A configuration directory should work as a Git repository that is committed and
restored on another machine, without Nix or other tooling. Two things prevent it:
secrets only live in the OS keychain or outside files, and paths are literal.

## Secrets encrypted with sops

A `sops` secret kind reads one value from a sops-encrypted file by running the
`sops` CLI:

```yaml
secrets:
  github:
    kind: sops
    file: ./secrets.yaml      # relative to the declaring fragment
    key: github.token         # omit for dotenv/binary files read whole
mcp:
  github:
    url: https://api.githubcopilot.com/mcp
    auth: !secret github
```

It runs `sops decrypt --extract '["github"]["token"]' -- <file>` without a shell,
under the time and output bounds of `command` secrets, and trims one trailing
newline. Keys may also be a list (`key: [github, token]`) when a segment contains
a dot. Segments are always map keys: list indexes are not supported, and a
segment cannot contain `][`, which sops's path syntax cannot escape. Like other
secrets, a sops secret can be top-level, inline in an MCP entry's `auth`, or used
through `!secret`. Its file resolves like a `!file` path: `~` is allowed, and a
package's file must stay inside the package.

sops chooses the decryption key (age, PGP, KMS, Vault) from its own configuration,
so `.sops.yaml` creation rules and `sops secrets.yaml` editing work unchanged. It
receives the environment baseline of `command` secrets plus an allowlist of what
key discovery reads: `SOPS_*`, `AGE_*`, the cloud and Vault credential prefixes
(`AWS_`, `GOOGLE_`, `CLOUDSDK_`, `AZURE_`, `VAULT_`), `XDG_CONFIG_HOME` for the
default age key, GnuPG's home and agent variables, and proxies. Inheriting the
whole daemon environment was rejected: it would hand sops variables unrelated to
decryption, which `command` secrets deliberately withhold, while prefixes cover
every key source sops supports.

The daemon caches decrypted values in memory, by file and key, until the file's
modification time or size changes; values are never written to disk or logged.
A missing `sops` executable, decryption failure, or missing key marks dependent
servers `needs-auth` with sops's first error line, `agx secret status` reports the
secret `unavailable` with that line, and a launch whose harness needs the secret
fails naming the secret and file. Passthrough commands decrypt without the daemon,
as for `command` and `keychain` secrets. `agx secret set` and `login` reject sops
secrets with a hint to edit the file with `sops`.

Validation fails when the file is missing: the configuration is portable only with
the encrypted file committed beside it, so a missing one is a broken checkout, not
a runtime state.

Restoring a machine is then: clone the repository as the configuration directory,
provide the sops key, `agx install --frozen`, start the daemon, and run `agx
secret login` for OAuth servers. OAuth tokens remain runtime state in the keychain
because refresh tokens rotate; only client registration belongs in configuration.

Home-relative paths are covered by [ADR 0011](0011-straightforward-configuration.md).

Rejected: linking the sops Go library, which pulls cloud SDKs into the binary;
an agx-specific age format, which duplicates sops; `${VAR}` interpolation, which
starts a templating language (`!env` covers caller-supplied values).

## Addendum: age key from a secret

A sops secret may set `age_key`, an injected value (`!secret`, `!env`, `!file`)
that agx passes to that sops run as `SOPS_AGE_KEY`, replacing inherited
`SOPS_AGE_KEY`, `SOPS_AGE_KEY_FILE`, and `SOPS_AGE_KEY_CMD`. Keeping the identity
in a `keychain` secret spares a daemon started by a service manager a key file or
variable. Literal keys are rejected: a key beside the file it decrypts defeats
encryption. Validation rejects `oauth` age keys and chains of sops secrets whose
age keys lead back to themselves. The decrypted-value cache is keyed by a hash of
the age key too, so changing the key decrypts again; the key itself stays in
memory only.
