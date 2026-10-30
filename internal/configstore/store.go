package configstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"reflect"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/storage"
	"github.com/tarik02-org/agx/internal/storage/statedb"
)

func Read(ctx context.Context, state, root string) (*config.Local, error) {
	local := config.EmptyLocal()
	if state == "" {
		return local, nil
	}
	root, err := platform.Canonical(root)
	if err != nil {
		return nil, err
	}
	return readOwner(ctx, state, root)
}

func readOwner(ctx context.Context, state, root string) (*config.Local, error) {
	local := config.EmptyLocal()
	db, err := storage.OpenExisting(ctx, storage.State(state))
	if errors.Is(err, fs.ErrNotExist) {
		return local, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	return readLocalRows(ctx, statedb.New(tx), root)
}

// Write commits changed definitions, rules and locks with a revision check.
func Write(ctx context.Context, state, root string, local *config.Local) (int64, error) {
	if state == "" {
		return 0, errors.New("local configuration needs a state directory")
	}
	root, err := platform.Canonical(root)
	if err != nil {
		return 0, err
	}
	return writeOwner(ctx, state, root, local, nil)
}

func writeOwner(
	ctx context.Context,
	state, root string,
	local *config.Local,
	record func(*statedb.Queries) error,
) (int64, error) {
	db, err := storage.Open(ctx, storage.State(state))
	if err != nil {
		return 0, err
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	q := statedb.New(tx)
	previous, err := readLocalRows(ctx, q, root)
	if err != nil {
		return 0, err
	}
	if previous.Revision != local.Revision {
		return 0, config.ErrConflict
	}
	if err := q.EnsureConfigRoot(ctx, root); err != nil {
		return 0, err
	}
	for _, kind := range config.Kinds {
		old, next := localObject(previous.Document[string(kind)]), localObject(local.Document[string(kind)])
		for id := range old {
			if _, found := next[id]; !found {
				if err := q.DeleteLocalDefinition(
					ctx,
					statedb.DeleteLocalDefinitionParams{Root: root, Key: definitionKey(kind, id)},
				); err != nil {
					return 0, err
				}
			}
		}
		for id, value := range next {
			if reflect.DeepEqual(old[id], value) {
				continue
			}
			key := definitionKey(kind, id)
			if err := q.DeleteLocalDefinition(
				ctx,
				statedb.DeleteLocalDefinitionParams{Root: root, Key: key},
			); err != nil {
				return 0, err
			}
			if err := writeLocalDefinition(
				ctx,
				q,
				root,
				key,
				kind,
				id,
				sql.NullString{},
				sql.NullInt64{},
				localObject(value),
			); err != nil {
				return 0, err
			}
		}
	}
	oldScopes, nextScopes := localArray(previous.Document["scopes"]), localArray(local.Document["scopes"])
	for i := range max(len(oldScopes), len(nextScopes)) {
		if i < len(oldScopes) && i < len(nextScopes) && reflect.DeepEqual(oldScopes[i], nextScopes[i]) {
			continue
		}
		if err := q.DeleteLocalScope(ctx, statedb.DeleteLocalScopeParams{Root: root, Position: int64(i)}); err != nil {
			return 0, err
		}
		if i < len(nextScopes) {
			if err := writeLocalScope(ctx, q, root, int64(i), localObject(nextScopes[i])); err != nil {
				return 0, err
			}
		}
	}
	if err := writeLocalLocks(ctx, q, root, previous.Lock.Packages, local.Lock.Packages); err != nil {
		return 0, err
	}
	revision, err := q.AdvanceConfigRevision(
		ctx,
		statedb.AdvanceConfigRevisionParams{Root: root, Revision: local.Revision},
	)
	if err != nil {
		return 0, err
	}
	if record != nil {
		if err := record(q); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return revision, nil
}

func Revision(state, root string) string {
	root, err := platform.Canonical(root)
	if err != nil {
		return "error:" + err.Error()
	}
	db, err := storage.OpenExisting(context.Background(), storage.State(state))
	if errors.Is(err, fs.ErrNotExist) {
		return "0"
	}
	if err != nil {
		return "error:" + err.Error()
	}
	defer func() { _ = db.Close() }()
	revision, err := statedb.New(db).GetConfigRevision(context.Background(), root)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "error:" + err.Error()
	}
	return fmt.Sprint(revision)
}
