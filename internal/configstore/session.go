package configstore

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/secretstore"
	"github.com/tarik02-org/agx/internal/storage"
	"github.com/tarik02-org/agx/internal/storage/statedb"
)

// A session owns a private configuration document in the same relational schema.
// NUL separates its key from filesystem roots, which can never contain it.
func sessionOwner(root, launch string) string { return "session\x00" + root + "\x00" + launch }

func ReadSession(
	ctx context.Context,
	state, root, launch string,
	key [32]byte,
) (*config.Local, map[config.ID]map[string]string, error) {
	local := config.EmptyLocal()
	if state == "" {
		return local, nil, nil
	}
	db, err := storage.OpenExisting(ctx, storage.State(state))
	if errors.Is(err, fs.ErrNotExist) {
		return local, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	q := statedb.New(tx)
	owner := sessionOwner(root, launch)
	local, err = readLocalRows(ctx, q, owner)
	if err != nil {
		return nil, nil, err
	}
	rows, err := q.ListSessionEnvironment(ctx, owner)
	if err != nil {
		return nil, nil, err
	}
	inputs := map[config.ID]map[string]string{}
	for _, row := range rows {
		value, err := secretstore.Unseal(key[:], sessionInputKey(owner, row.Mcp, row.Name), row.Ciphertext)
		if err != nil {
			return nil, nil, err
		}
		if inputs[config.ID(row.Mcp)] == nil {
			inputs[config.ID(row.Mcp)] = map[string]string{}
		}
		inputs[config.ID(row.Mcp)][row.Name] = value
	}
	return local, inputs, nil
}

func sessionInputKey(owner, mcp, name string) string {
	return owner + "\x00" + mcp + "\x00" + name
}

func WriteSession(
	ctx context.Context,
	state, root, launch string,
	local *config.Local,
	inputs map[config.ID]map[string]string,
	key [32]byte,
) (int64, error) {
	if state == "" {
		return 0, errors.New("session configuration needs a state directory")
	}
	owner := sessionOwner(root, launch)
	return writeOwner(ctx, state, owner, local, func(q *statedb.Queries) error {
		if err := q.PutSessionConfiguration(ctx, statedb.PutSessionConfigurationParams{
			Configuration: root, Launch: launch, Owner: owner,
		}); err != nil {
			return err
		}
		if err := q.DeleteSessionEnvironment(ctx, owner); err != nil {
			return err
		}
		for id, values := range inputs {
			for name, value := range values {
				ciphertext, err := secretstore.Seal(key[:], sessionInputKey(owner, string(id), name), value)
				if err != nil {
					return err
				}
				if err := q.PutSessionEnvironment(
					ctx,
					statedb.PutSessionEnvironmentParams{
						Owner:      owner,
						Mcp:        string(id),
						Name:       name,
						Ciphertext: ciphertext,
					},
				); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func DeleteSession(ctx context.Context, state, root, launch string) error {
	if state == "" {
		return errors.New("session configuration needs a state directory")
	}
	db, err := storage.Open(ctx, storage.State(state))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return statedb.New(db).DeleteConfigOwner(ctx, sessionOwner(root, launch))
}
