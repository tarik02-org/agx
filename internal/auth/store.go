package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/zalando/go-keyring"

	"github.com/tarik02-org/agx/internal/securefs"
)

// storeVersion is the format version of the secret store file.
const storeVersion = 1

// errNotStored reports that the store holds no value under a key.
var errNotStored = errors.New("not stored")

// Store keeps keychain secrets and OAuth credentials encrypted in a file of the state directory,
// under one key that the OS keychain holds. The keychain is asked once per process instead of
// once per secret, values have no size limit (Windows Credential Manager refuses secrets over
// 2560 bytes), and every change is one locked, atomic write, so processes sharing the store see
// the latest refresh token. There is no plaintext fallback: without the keychain, the store
// cannot be read or written.
type Store struct {
	path string
	// mu serializes this process's access and guards key; the file lock serializes processes.
	mu  sync.Mutex
	key []byte
}

type storeFile struct {
	Version int `json:"version"`
	// Records maps a key to base64 of an AES-256-GCM nonce and ciphertext, sealed with the key
	// as additional data, so a record cannot be moved under another key.
	Records map[string]string `json:"records"`
}

// OpenStore returns the store in state directory dir; nothing is read until it is used.
func OpenStore(dir string) *Store { return &Store{path: filepath.Join(dir, "secrets.json")} }

// keyItem names the keychain item holding the store's key. It covers the store's path, so
// isolated state directories do not share a key.
func (s *Store) keyItem() string {
	sum := sha256.Sum256([]byte(s.path))
	return "store-key:" + hex.EncodeToString(sum[:8])
}

// Get returns the value under k. A value only the keychain holds, as agx stored values before
// the store existed, moves into the store.
func (s *Store) Get(k string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	f, err := s.read()
	if err != nil {
		return "", err
	}
	if sealed, ok := f.Records[k]; ok {
		key, err := s.loadKey(f, false)
		if err != nil {
			return "", err
		}
		if key == nil {
			// The key was lost and the file moved aside.
			return "", errNotStored
		}
		return unseal(key, k, sealed)
	}
	legacy, err := keychainGet(k)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", errNotStored
	}
	if err != nil {
		return "", err
	}
	if err := s.put(f, k, legacy); err != nil {
		return "", err
	}
	// A failure leaves a duplicate in the keychain, which the store's value shadows.
	if err := keychainDelete(k); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		slog.Warn("cannot remove a keychain item moved into the secret store", "error", err)
	}
	return legacy, nil
}

// Set stores value under k.
func (s *Store) Set(k, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	f, err := s.read()
	if err != nil {
		return err
	}
	return s.put(f, k, value)
}

// Delete removes the value under k, from the keychain too when it was never moved; a missing
// value is not an error.
func (s *Store) Delete(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	f, err := s.read()
	if err != nil {
		return err
	}
	if _, ok := f.Records[k]; ok {
		delete(f.Records, k)
		if err := s.write(f); err != nil {
			return err
		}
	}
	if err := keychainDelete(k); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

// put seals value under k and writes the file; s.mu and the file lock must be held.
func (s *Store) put(f *storeFile, k, value string) error {
	key, err := s.loadKey(f, true)
	if err != nil {
		return err
	}
	sealed, err := seal(key, k, value)
	if err != nil {
		return err
	}
	f.Records[k] = sealed
	return s.write(f)
}

// loadKey returns the store's key, asking the keychain only the first time, and creates it when
// create is set and none exists. A file whose key the keychain lost, as after a keychain reset,
// cannot be read again: it moves aside and f empties, and loadKey returns nil without create.
// s.mu must be held.
func (s *Store) loadKey(f *storeFile, create bool) ([]byte, error) {
	if s.key != nil {
		return s.key, nil
	}
	encoded, err := keychainGet(s.keyItem())
	switch {
	case err == nil:
		key, decodeErr := base64.StdEncoding.DecodeString(encoded)
		if decodeErr != nil || len(key) != 32 {
			return nil, fmt.Errorf("the secret store key in the OS keychain is damaged")
		}
		s.key = key
		return key, nil
	case !errors.Is(err, keyring.ErrNotFound):
		return nil, err
	}
	if len(f.Records) > 0 {
		aside := s.path + ".lost-" + time.Now().UTC().Format("20060102T150405Z")
		if err := os.Rename(s.path, aside); err != nil {
			return nil, fmt.Errorf(
				"the OS keychain lost the secret store key, and %s cannot be moved aside: %w",
				s.path,
				err,
			)
		}
		slog.Warn(
			"the OS keychain lost the secret store key; its secrets must be stored and logged in again",
			"moved", aside,
		)
		f.Records = map[string]string{}
	}
	if !create {
		return nil, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := keychainSet(s.keyItem(), base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, err
	}
	s.key = key
	return key, nil
}

// lock takes the store's file lock, which other agx processes respect.
func (s *Store) lock() (func(), error) {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return nil, err
	}
	lock := flock.New(s.path + ".lock")
	if err := lock.Lock(); err != nil {
		return nil, fmt.Errorf("cannot lock the secret store: %w", err)
	}
	return func() { _ = lock.Unlock() }, nil
}

func (s *Store) read() (*storeFile, error) {
	f := &storeFile{Version: storeVersion, Records: map[string]string{}}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	if err := securefs.CheckFormat(s.path, b, storeVersion); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, f); err != nil || f.Version != storeVersion {
		return nil, fmt.Errorf("%s does not decode; move it aside to start an empty secret store", s.path)
	}
	if f.Records == nil {
		f.Records = map[string]string{}
	}
	return f, nil
}

func (s *Store) write(f *storeFile) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return securefs.Write(s.path, append(b, '\n'), 0o600)
}

func seal(key []byte, k, value string) (string, error) {
	aead, err := gcm(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(value), []byte(k))), nil
}

func unseal(key []byte, k, sealed string) (string, error) {
	aead, err := gcm(key)
	if err != nil {
		return "", err
	}
	b, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(b) < aead.NonceSize() {
		return "", fmt.Errorf("the secret store entry for %s is damaged", k)
	}
	value, err := aead.Open(nil, b[:aead.NonceSize()], b[aead.NonceSize():], []byte(k))
	if err != nil {
		return "", fmt.Errorf("the secret store entry for %s does not decrypt with the store key", k)
	}
	return string(value), nil
}

func gcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
