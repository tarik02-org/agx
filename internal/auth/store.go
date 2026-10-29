package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/gofrs/flock"
	"github.com/zalando/go-keyring"

	"github.com/tarik02-org/agx/internal/securefs"
)

// masterKeyItem names the OS keychain item holding the secret store's key.
const masterKeyItem = "master-key"

// errNotStored reports that the store holds no value under a key.
var errNotStored = errors.New("not stored")

// Store keeps keychain secrets and OAuth credentials encrypted in a file of the state directory,
// under one master key that the OS keychain holds. The keychain is asked once per process, values
// have no size limit, and every change is one locked, atomic write, so processes sharing the store
// see each other's refreshed tokens. Without the keychain the store can be neither read nor
// written; there is no plaintext fallback.
type Store struct {
	path string
	// mu serializes this process's access and guards key; the file lock serializes processes.
	mu  sync.Mutex
	key []byte
}

type storeFile struct {
	// Records maps a key to base64 of an AES-256-GCM nonce and ciphertext, sealed with the key as
	// additional data, so a record cannot be moved under another key.
	Records map[string]string `json:"records"`
}

// OpenStore returns the store in state directory dir; nothing is read until it is used.
func OpenStore(dir string) *Store { return &Store{path: filepath.Join(dir, "secrets.json")} }

// Get returns the value under k, or errNotStored.
func (s *Store) Get(k string) (string, error) {
	var value string
	err := s.locked(func(f *storeFile) (bool, error) {
		sealed, ok := f.Records[k]
		if !ok {
			return false, errNotStored
		}
		key, err := s.masterKey(f)
		if err != nil {
			return false, err
		}
		value, err = unseal(key, k, sealed)
		return false, err
	})
	return value, err
}

// Set stores value under k.
func (s *Store) Set(k, value string) error {
	return s.locked(func(f *storeFile) (bool, error) {
		key, err := s.masterKey(f)
		if err != nil {
			return false, err
		}
		f.Records[k], err = seal(key, k, value)
		return err == nil, err
	})
}

// Delete removes the value under k; a missing value is not an error.
func (s *Store) Delete(k string) error {
	return s.locked(func(f *storeFile) (bool, error) {
		_, ok := f.Records[k]
		delete(f.Records, k)
		return ok, nil
	})
}

// locked runs op on the store's contents while holding its locks, and writes them when op
// reports a change.
func (s *Store) locked(op func(*storeFile) (bool, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := securefs.EnsureDir(filepath.Dir(s.path)); err != nil {
		return err
	}
	lock := flock.New(s.path + ".lock")
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("cannot lock the secret store: %w", err)
	}
	defer func() { _ = lock.Unlock() }()
	f, err := s.read()
	if err != nil {
		return err
	}
	changed, err := op(f)
	if err != nil || !changed {
		return err
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return securefs.WritePrivate(s.path, append(b, '\n'))
}

// masterKey returns the store's key, asking the keychain only the first time, and creates it for
// a store that holds nothing yet. s.mu must be held.
func (s *Store) masterKey(f *storeFile) ([]byte, error) {
	if s.key != nil {
		return s.key, nil
	}
	encoded, err := keychainGet(masterKeyItem)
	switch {
	case err == nil:
		key, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(key) != 32 {
			return nil, errors.New("the secret store's master key in the OS keychain is damaged")
		}
		s.key = key
		return key, nil
	case !errors.Is(err, keyring.ErrNotFound):
		return nil, err
	case len(f.Records) > 0:
		return nil, fmt.Errorf(
			"the OS keychain has no master key for %s, so its secrets cannot be read; delete it to start over",
			s.path,
		)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := keychainSet(masterKeyItem, base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, err
	}
	s.key = key
	return key, nil
}

func (s *Store) read() (*storeFile, error) {
	f := &storeFile{Records: map[string]string{}}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, f); err != nil {
		return nil, fmt.Errorf("%s does not decode", s.path)
	}
	if f.Records == nil {
		f.Records = map[string]string{}
	}
	return f, nil
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
		return "", errors.New("a secret store entry is damaged")
	}
	value, err := aead.Open(nil, b[:aead.NonceSize()], b[aead.NonceSize():], []byte(k))
	if err != nil {
		return "", errors.New("a secret store entry does not decrypt with the master key")
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
