package configstore

import (
	"context"
	"reflect"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/storage/statedb"
)

func readLocalLocks(ctx context.Context, q *statedb.Queries, root string, out *config.Local) error {
	locks, err := q.ListLocalPackageLocks(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range locks {
		out.Lock.Packages[config.ID(r.ID)] = config.LockedPackage{
			Source:   config.Package{Path: r.Path, Git: r.Git, Ref: r.Ref, Hash: r.SourceHash, Subdir: r.Subdir},
			Revision: r.Revision,
			Hash:     r.Hash,
		}
	}
	deps, err := q.ListLocalPackageDependencies(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range deps {
		lock := out.Lock.Packages[config.ID(r.Package)]
		lock.Dependencies = append(lock.Dependencies, config.ID(r.Dependency))
		out.Lock.Packages[config.ID(r.Package)] = lock
	}
	return nil
}

func writeLocalLocks(
	ctx context.Context,
	q *statedb.Queries,
	root string,
	previous, next map[config.ID]config.LockedPackage,
) error {
	for id, old := range previous {
		if value, found := next[id]; found && reflect.DeepEqual(old, value) {
			continue
		}
		if err := q.DeleteLocalPackageLock(
			ctx,
			statedb.DeleteLocalPackageLockParams{Root: root, ID: string(id)},
		); err != nil {
			return err
		}
	}
	for id, lock := range next {
		if old, found := previous[id]; found && reflect.DeepEqual(old, lock) {
			continue
		}
		if err := q.InsertLocalPackageLock(
			ctx,
			statedb.InsertLocalPackageLockParams{
				Root:       root,
				ID:         string(id),
				Path:       lock.Source.Path,
				Git:        lock.Source.Git,
				Ref:        lock.Source.Ref,
				SourceHash: lock.Source.Hash,
				Subdir:     lock.Source.Subdir,
				Revision:   lock.Revision,
				Hash:       lock.Hash,
			},
		); err != nil {
			return err
		}
		for i, dependency := range lock.Dependencies {
			if err := q.InsertLocalPackageDependency(
				ctx,
				statedb.InsertLocalPackageDependencyParams{
					Root:       root,
					Package:    string(id),
					Position:   int64(i),
					Dependency: string(dependency),
				},
			); err != nil {
				return err
			}
		}
	}
	return nil
}
