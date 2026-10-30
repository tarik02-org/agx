package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"reflect"

	"github.com/tarik02-org/agx/internal/storage/statedb"

	"go.yaml.in/yaml/v4"

	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/storage"
)

// Local contains definitions and rules managed by agx, scoped to one root configuration.
type Local struct {
	Document map[string]any
	Lock     Lock
	Revision int64
}

var ErrConflict = errors.New("local configuration changed; retry the command")

func EmptyLocal() *Local {
	return &Local{Document: map[string]any{}, Lock: Lock{Version: 1, Packages: map[ID]LockedPackage{}}}
}

func ReadLocal(ctx context.Context, state, root string) (*Local, error) {
	local := EmptyLocal()
	if state == "" {
		return local, nil
	}
	root, err := platform.Canonical(root)
	if err != nil {
		return nil, err
	}
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

// WriteLocal commits changed definitions, rules and locks with a revision check.
func WriteLocal(ctx context.Context, state, root string, local *Local) (int64, error) {
	if state == "" {
		return 0, errors.New("local configuration needs a state directory")
	}
	root, err := platform.Canonical(root)
	if err != nil {
		return 0, err
	}
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
		return 0, ErrConflict
	}
	if err := q.EnsureConfigRoot(ctx, root); err != nil {
		return 0, err
	}
	for _, kind := range Kinds {
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
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return revision, nil
}

func (l *Local) Clone() *Local {
	data, _ := json.Marshal(l)
	var clone Local
	_ = json.Unmarshal(data, &clone)
	return &clone
}

func (c *Config) PackageLock(p Package) (Lock, error) {
	if p.Local {
		return c.Local.Lock, nil
	}
	return ReadLock(p.Lockfile)
}

func (l *Local) decode(root string, builtins Builtins) (Document, positions, error) {
	data, err := yaml.Marshal(l.Document)
	if err != nil {
		return Document{}, nil, err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return Document{}, nil, err
	}
	if err := localTags(&node); err != nil {
		return Document{}, nil, err
	}
	document, at, err := decodeNode[Document](root, data, &node, FragmentSchema, builtins)
	var located *SourceError
	if errors.As(err, &located) {
		located.Local, located.Line = true, 0
	}
	for key, source := range at {
		source.Local, source.Line, source.Column = true, 0, 0
		at[key] = source
	}
	return document, at, err
}

func localRevision(state, root string) string {
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

// Export renders local definitions as a declarative YAML overlay.
func (l *Local) Export() ([]byte, error) {
	data, err := yaml.Marshal(l.Document)
	if err != nil {
		return nil, err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	if err := localTags(&node); err != nil {
		return nil, err
	}
	return yaml.Marshal(&node)
}
