package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"github.com/zalando/go-keyring"

	"github.com/tarik02-org/agx/internal/storage"
	"github.com/tarik02-org/agx/internal/storage/statedb"
)

// masterKeyItem names the OS keychain item holding the secret store's key.
const masterKeyItem = "master-key"

// errNotStored reports that the store holds no value under a key.
var errNotStored = errors.New("not stored")

// Store keeps encrypted secrets under a master key held by the OS keychain.
// Opening is lazy: unrelated commands never open the database or ask the keychain.
type Store struct {
	path string
	mu   sync.Mutex
	key  []byte
}

func OpenStore(dir string) *Store { return &Store{path: storage.State(dir)} }

func (s *Store) Get(k string) (string, error) {
	var value string
	err := s.locked(func(tx *sql.Tx) error {
		sealed, err := statedb.New(tx).GetSecret(context.Background(), k)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errNotStored
			}
			return err
		}
		key, err := s.masterKey(tx)
		if err != nil {
			return err
		}
		value, err = unseal(key, k, sealed)
		return err
	})
	return value, err
}

func (s *Store) Set(k, value string) error {
	return s.locked(func(tx *sql.Tx) error {
		key, err := s.masterKey(tx)
		if err != nil {
			return err
		}
		sealed, err := seal(key, k, value)
		if err != nil {
			return err
		}
		err = statedb.New(tx).PutSecret(context.Background(), statedb.PutSecretParams{Key: k, Ciphertext: sealed})
		return err
	})
}

func (s *Store) Delete(k string) error {
	return s.locked(func(tx *sql.Tx) error {
		return statedb.New(tx).DeleteSecret(context.Background(), k)
	})
}

func (s *Store) locked(op func(*sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	db, err := storage.Open(context.Background(), s.path)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := op(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// masterKey returns the store's key, asking the keychain only the first time, and creates it for
// a store that holds nothing yet. s.mu must be held.
func (s *Store) masterKey(tx *sql.Tx) ([]byte, error) {
	if s.key != nil {
		return s.key, nil
	}
	populated, err := statedb.New(tx).HasSecrets(context.Background())
	if err != nil {
		return nil, err
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
	case populated:
		return nil, fmt.Errorf(
			"the OS keychain has no master key for %s, so its secrets cannot be read; remove the stored credentials with agx secret logout, then store or log in again",
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
