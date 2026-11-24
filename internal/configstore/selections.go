package configstore

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/storage/statedb"
)

func writeLocalArgs(ctx context.Context, q *statedb.Queries, root, key string, args []any) error {
	if len(args) == 0 {
		return nil
	}
	first := localObject(args[0])
	_, values := first["values"]
	_, modes := first["modes"]
	if !values && !modes {
		args = []any{map[string]any{"values": args}}
	}
	for i, value := range args {
		group := localObject(value)
		if err := q.InsertLocalArgGroup(
			ctx,
			statedb.InsertLocalArgGroupParams{Root: root, Key: key, Position: int64(i)},
		); err != nil {
			return err
		}
		for j, value := range localArray(group["values"]) {
			kind, text, err := localValue(value)
			if err != nil {
				return err
			}
			if err := q.InsertLocalArgument(
				ctx,
				statedb.InsertLocalArgumentParams{
					Root:          root,
					Key:           key,
					GroupPosition: int64(i),
					Position:      int64(j),
					ValueKind:     kind,
					Value:         text,
				},
			); err != nil {
				return err
			}
		}
		for j, value := range localArray(group["modes"]) {
			if err := q.InsertLocalArgumentMode(
				ctx,
				statedb.InsertLocalArgumentModeParams{
					Root:          root,
					Key:           key,
					GroupPosition: int64(i),
					Position:      int64(j),
					Mode:          value.(string),
				},
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeLocalScope(ctx context.Context, q *statedb.Queries, root string, position int64, m map[string]any) error {
	if err := q.InsertLocalScope(
		ctx,
		statedb.InsertLocalScopeParams{Root: root, Position: position, Directory: localText(m, "directory")},
	); err != nil {
		return err
	}
	for i, value := range localArray(m["harnesses"]) {
		if err := q.InsertLocalScopeHarness(
			ctx,
			statedb.InsertLocalScopeHarnessParams{
				Root:          root,
				ScopePosition: position,
				Position:      int64(i),
				Name:          value.(string),
			},
		); err != nil {
			return err
		}
	}
	for _, action := range []string{"add", "remove"} {
		if err := writeLocalSelection(
			ctx,
			q,
			root,
			"",
			sql.NullInt64{Int64: position, Valid: true},
			action,
			localObject(m[action]),
		); err != nil {
			return err
		}
	}
	return nil
}

func writeLocalSelection(
	ctx context.Context,
	q *statedb.Queries,
	root, key string,
	scope sql.NullInt64,
	action string,
	selection map[string]any,
) error {
	for _, kind := range config.SelectionKinds {
		for i, value := range localArray(selection[string(kind)]) {
			var reference, inline sql.NullString
			if text, ok := value.(string); ok {
				reference = sql.NullString{String: text, Valid: true}
			} else {
				parent := sql.NullString{String: key, Valid: key != ""}
				base := key
				if scope.Valid {
					base = localScopeKey(scope.Int64)
				}
				inline = sql.NullString{
					String: base + "/" + action + "/" + string(kind) + "/" + strconv.Itoa(i),
					Valid:  true,
				}
				body := localObject(value)
				if err := writeLocalDefinition(
					ctx,
					q,
					root,
					inline.String,
					kind,
					localText(body, "id"),
					parent,
					scope,
					body,
				); err != nil {
					return err
				}
			}
			if scope.Valid {
				if err := q.InsertLocalScopeItem(
					ctx,
					statedb.InsertLocalScopeItemParams{
						Root:          root,
						ScopePosition: scope.Int64,
						Action:        action,
						Kind:          string(kind),
						Position:      int64(i),
						Reference:     reference,
						InlineKey:     inline,
					},
				); err != nil {
					return err
				}
			} else {
				if err := q.InsertLocalDefinitionItem(
					ctx,
					statedb.InsertLocalDefinitionItemParams{
						Root:      root,
						Key:       key,
						Action:    action,
						Kind:      string(kind),
						Position:  int64(i),
						Reference: reference,
						InlineKey: inline,
					},
				); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func localScopeKey(position int64) string { return "/scopes/" + strconv.FormatInt(position, 10) }
