# Keep configuration portable: sops secrets and home-relative paths

Status: proposed.

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

It runs `sops decrypt --extract '["github"]["token"]' <file>`. Keys may also be a
list (`key: [github, token]`) when a segment contains a dot. sops chooses the
decryption key (age, PGP, KMS, Vault) from its own configuration, so `.sops.yaml`
creation rules and `sops secrets.yaml` editing work unchanged. The daemon caches
decrypted values in memory until the file's modification time or size changes;
values are never written to disk. A missing `sops` executable or decryption
failure marks dependent servers `needs-auth` with sops's first error line.

Restoring a machine is then: clone the repository as the configuration directory,
provide the sops key, `agx install --frozen`, start the daemon, and run `agx
secret login` for OAuth servers. OAuth tokens remain runtime state in the keychain
because refresh tokens rotate; only client registration belongs in configuration.

## Home-relative paths

Paths in configuration accept a leading `~` or `~/`, expanded to the home
directory of the user running agx: scope directories, imports, local package
sources, resource paths, `!file`, sops files, and executables. `~user` is not
supported. Machine-specific settings belong in an ignored fragment such as
`agx.d/50-local.yaml`.

Rejected: linking the sops Go library, which pulls cloud SDKs into the binary;
an agx-specific age format, which duplicates sops; `${VAR}` interpolation, which
starts a templating language (`!env` covers caller-supplied values).
