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

Values live in `secrets.json` in the state directory. The file is JSON written
atomically with mode 0600 (a private ACL on Windows), and every read-modify-write holds
`secrets.json.lock`. Each record is sealed with AES-256-GCM under one 256-bit key, with the
record's key as additional data, so a record cannot be moved under another key. The OS keychain
holds only that key, as the single item `master-key`. A process asks the keychain for it once and
keeps it in memory.

- **Lost key:** a store with records whose master key the keychain no longer has, as after a
  keychain reset, cannot be read; agx says so and names the file to delete to start over.
- **No keychain:** the store fails as the keychain items did; nothing is ever stored in
  plaintext.
- **Earlier keychain items** are not read; secrets stored before need storing or logging in
  again.

## Consequences

The key's keychain item is the only thing that protects every stored secret, so any process that
reads the key reads them all. That is the same boundary as before in practice: the daemon already
holds the decrypted values, and keychain items of one user are open to that user's processes
once unlocked. The file can be backed up or inspected without exposing values.

Rejected:
- Splitting long values across several keychain items: it keeps a prompt per item and the race
  between processes.
- A key derived from a passphrase: the daemon must start without a person present.
