# Keep stored secrets in an encrypted file under one keychain key

Status: accepted. Changes where `keychain` secrets and OAuth credentials live; "no plaintext
fallback" ([security](../security.md)) still holds.

agx stored every `keychain` secret and every OAuth credential as its own OS keychain item. That
ran into three problems:

- **Size:** Windows Credential Manager refuses secrets over 2560 bytes. A refreshed OAuth
  credential that grew past that could not be saved, lived in memory only, and was lost with
  the next daemon restart, together with a refresh token the server had already rotated.
- **Prompts:** macOS asks once per item when the agx binary changes, which happens on every Nix
  rebuild, and Linux asks the Secret Service on every use.
- **Consistency:** two processes refreshing one credential each read and wrote their own item
  with no lock, so one could overwrite the refresh token the other had rotated to.

## Decision

Values live in `secrets.json` in the state directory. The file is versioned JSON written
atomically with mode 0600 (a private ACL on Windows), and every read-modify-write holds
`secrets.json.lock`. Each record is sealed with AES-256-GCM under one 256-bit key, with the
record's key as additional data, so a record cannot be moved under another key. The OS keychain
holds only that key, as the item `store-key:<digest of the store path>`, so isolated state
directories (`AGX_STATE`, `--cache`) do not share one. A process asks the keychain for the key
once and keeps it in memory.

- **Migration:** a value found only in the keychain, under its old item name, moves into the
  file on its first read, and the item is deleted.
- **Lost key:** when the file has records but the keychain has no key, as after a keychain
  reset, the file moves aside to `secrets.json.lost-<UTC time>` and the store starts empty;
  its secrets read as not stored, so `agx secret status` asks for `agx secret set` or `login`.
- **No keychain:** the store fails as the keychain items did; nothing is ever stored in
  plaintext.

## Consequences

The key's keychain item is the only thing that protects every stored secret, so any process that
reads the key reads them all. That is the same boundary as before in practice: the daemon already
holds the decrypted values, and keychain items of one user are open to that user's processes
once unlocked. The file can be backed up or inspected without exposing values.

Rejected:
- Splitting long values across several keychain items: it keeps a prompt per item and the race
  between processes.
- A key derived from a passphrase: the daemon must start without a person present.
