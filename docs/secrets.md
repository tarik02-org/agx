# Secrets

Keep API keys and tokens out of your configuration files. agx reads them when a
harness launches or an MCP server starts, and never writes them to its files,
output, or logs.

## Using values from elsewhere

Three YAML tags insert a value at launch time:

```yaml
mcp:
  docs:
    url: https://docs.example.com/mcp
    headers:
      X-Api-Key: !secret docs-key        # a secret defined below
  db:
    command: [~/bin/db-mcp]
    environment:
      DB_URL: !env DATABASE_URL          # from the shell you launch from
harnesses:
  codex-work:
    base: codex
    environment:
      OPENAI_API_KEY: !secret work-openai
    args: [--instructions, !file ./prompt.txt]   # a file's contents

secrets:
  docs-key: {kind: keychain}
  work-openai: {kind: command, command: [pass, show, openai/work]}
```

| Tag | Value |
| --- | --- |
| `!secret <id>` | a secret from the `secrets` section |
| `!env <NAME>` | an environment variable of the shell you launch from; empty counts as unset |
| `!file <path>` | a file's contents without trailing newlines, up to 1 MiB; relative to the configuration file |

Tags work in:

- MCP server `headers`, `environment`, and `auth`;
- a sops secret's `age_key`;
- harness `environment`;
- harness `args`, `!file` only: arguments are visible to other processes, so
  `!secret` and `!env` are rejected there. Pass secrets through `environment`.

A missing variable or failing secret stops the launch with an error that names
it and where it is used, never its value. `agx explain` shows the tags as
written, such as `!secret docs-key`. Rotating a secret or editing a `!file`
takes effect at the next launch; nothing needs reinstalling.

`!env` only passes the variables your configuration names: agx never hands your
whole shell environment to the daemon.

## Kinds of secrets

```yaml
secrets:
  cpa-key:    {kind: command, command: [pass, show, cpa]}
  docs-key:   {kind: keychain}
  github:     {kind: sops, file: ./secrets.yaml, key: github.token}
  team-login: {kind: oauth, scopes: [read]}
```

| Kind | Where the value comes from |
| --- | --- |
| `command` | the trimmed output of a command, run with a minimal environment and a time limit; the daemon reuses it for a minute, or until configuration changes |
| `keychain` | the OS keychain; store it with `agx secret set <id>`, which prompts or reads stdin |
| `sops` | a value in a [sops](https://getsops.io)-encrypted file; see [below](#sops-encrypted-files) |
| `oauth` | an OAuth login; only for an MCP server's `auth`, see [MCP servers](mcp.md#logging-in) |

Store a keychain secret by typing it at the prompt, which does not echo it, or
by piping it in; either way it never appears in your shell history or process
list:

```sh
agx secret set docs-key
pbpaste | agx secret set docs-key
agx secret set docs-key < ~/Downloads/token.txt
```

For a value in an environment variable or a file, such as one decrypted by
sops-nix or agenix under `/run`, use `!env` or `!file` directly; no secret is
needed.

An MCP server with `auth: keychain` stores its token the same way, under the
server's ID. That is how the GitHub MCP server is set up, with a personal
access token: see [MCP servers](mcp.md#github).

agx keeps these values, and OAuth logins, encrypted in `secrets.json` in its
state directory. The OS keychain holds only the key, so agx asks it once per
process, whatever the number or size of the secrets. If the keychain loses the
key, for example after a keychain reset, the file cannot be read: delete it and
store the secrets again.

There is no plaintext fallback: when the OS keychain is unavailable, keychain
and OAuth secrets fail with an error.

## Commands

```sh
agx secret status            # every secret's kind and state, never values
agx secret status docs-key
agx secret set docs-key < token.txt
agx secret login team-login  # OAuth: a top-level secret, or a server's own login by its ID
agx secret logout docs-key   # delete the stored value or OAuth tokens
agx secret rotate team-login # renew an OAuth access token now
```

`agx secret rotate` renews an OAuth secret's access token with its refresh
token, whether or not it expired, and prints when the new one expires. agx
renews tokens on its own when they expire or a server rejects them; rotating is
for replacing a token you suspect leaked, or for checking that the refresh
token still works. When the authorization server refuses the refresh token, the
command fails with `needs_login`; when the server cannot be reached, with
`network`.

Secret IDs are the IDs under `secrets`, and for an MCP server whose `auth` is
written inline or omitted, the server's ID.

Logging in or out closes the affected MCP servers' sessions, so the next request
uses the new credential.

## sops-encrypted files

A `sops` secret keeps credentials encrypted inside your configuration
directory, so the whole directory can live in Git:

```yaml
secrets:
  github:
    kind: sops
    file: ./secrets.yaml         # relative to this configuration file
    key: github.token            # omit to use the whole decrypted file
  dotted:
    kind: sops
    file: ./secrets.yaml
    key: [api.example.com, token]   # a list when a key contains a dot
```

Edit the values with `sops secrets.yaml` as usual; `agx secret set` and `login`
do not apply. agx needs the `sops` command (3.9 or later) on `PATH`. Decrypted
values stay in memory only, until the file changes.

sops finds its key the way it always does: your `.sops.yaml`, an age key at
`~/.config/sops/age/keys.txt` (`~/Library/Application Support/sops/age/keys.txt`
on macOS), `SOPS_AGE_KEY_FILE`, PGP, or a cloud KMS. For launches, sops runs
inside the daemon, so those variables must be in the environment of the shell
that started the daemon.

### Keeping the age key in the keychain

`age_key` hands sops its age identity from another secret instead, for example
the OS keychain, so no key file is needed on disk:

```yaml
secrets:
  age: {kind: keychain}
  github: {kind: sops, file: ./secrets.yaml, key: github.token, age_key: !secret age}
```

```sh
agx secret set age < ~/.config/sops/age/keys.txt
```

`age_key` takes any value: `!secret`, `!env`, `!file`, or the key itself. To
have sops run a command for the key instead, leave `age_key` out and set
`SOPS_AGE_KEY_CMD` in the daemon's environment.

When decryption fails, `agx secret status` shows the secret as `unavailable`
with sops's error, and MCP servers using it report `needs-auth`.
