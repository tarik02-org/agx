package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/storage/statedb"
)

type localDefinition struct {
	kind config.Kind
	body map[string]any
}

type localGroup struct {
	key      string
	position int64
}

func definitionKey(kind config.Kind, id string) string {
	return "/" + string(kind) + "/" + strings.NewReplacer("~", "~0", "/", "~1").Replace(id)
}
func localObject(v any) map[string]any { m, _ := v.(map[string]any); return m }
func localArray(v any) []any {
	switch v := v.(type) {
	case []any:
		return v
	case []string:
		out := make([]any, len(v))
		for i, s := range v {
			out[i] = s
		}
		return out
	default:
		return nil
	}
}

func localText(m map[string]any, key string) string { s, _ := m[key].(string); return s }
func localOptionalText(m map[string]any, key string) sql.NullString {
	s, found := m[key].(string)
	return sql.NullString{String: s, Valid: found}
}

func readLocalText(m map[string]any, key string, value sql.NullString) {
	if value.Valid {
		m[key] = value.String
	}
}

func localFlag(m map[string]any, key string) bool { b, _ := m[key].(bool); return b }
func localMap(m map[string]any, key string) map[string]any {
	child := localObject(m[key])
	if child == nil {
		child = map[string]any{}
		m[key] = child
	}
	return child
}
func localAppend(m map[string]any, key string, value any) { m[key] = append(localArray(m[key]), value) }
func localStrings(m map[string]any, fields ...string) {
	for i := 0; i < len(fields); i += 2 {
		if fields[i+1] != "" {
			m[fields[i]] = fields[i+1]
		}
	}
}

func localInteger(v any) int64 {
	switch v := v.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func localJSON(v any) (sql.NullString, error) {
	if v == nil {
		return sql.NullString{}, nil
	}
	b, err := json.Marshal(v)
	return sql.NullString{String: string(b), Valid: err == nil}, err
}

func readLocalJSON(m map[string]any, key string, v sql.NullString) error {
	if !v.Valid {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(v.String), &value); err != nil {
		return err
	}
	m[key] = value
	return nil
}

func localValue(v any) (string, string, error) {
	if text, ok := v.(string); ok {
		return "literal", text, nil
	}
	if value := localObject(v); len(value) == 1 {
		for _, kind := range []string{"env", "file", "secret"} {
			if text, ok := value["$"+kind].(string); ok {
				return kind, text, nil
			}
		}
	}
	return "", "", fmt.Errorf("expected a literal or an injected value, got %T", v)
}

func storedLocalValue(kind, value string) any {
	if kind == "literal" {
		return value
	}
	return map[string]any{"$" + kind: value}
}

func readLocalRows(ctx context.Context, q *statedb.Queries, root string) (*config.Local, error) {
	out := config.EmptyLocal()
	revision, err := q.GetConfigRevision(ctx, root)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.Revision = revision
	rows, err := q.ListLocalDefinitions(ctx, root)
	if err != nil {
		return nil, err
	}
	definitions := map[string]localDefinition{}
	for _, row := range rows {
		body := map[string]any{}
		if row.Override {
			body["override"] = true
		}
		if row.Parent.Valid || row.Scope.Valid {
			if row.Name != "" {
				body["id"] = row.Name
			}
		} else {
			localMap(out.Document, row.Kind)[row.Name] = body
		}
		definitions[row.Key] = localDefinition{kind: config.Kind(row.Kind), body: body}
	}
	if err := readLocalResources(ctx, q, root, definitions); err != nil {
		return nil, err
	}
	if err := readLocalChildren(ctx, q, root, definitions); err != nil {
		return nil, err
	}
	if err := readLocalSelections(ctx, q, root, out, definitions); err != nil {
		return nil, err
	}
	if err := readLocalLocks(ctx, q, root, out); err != nil {
		return nil, err
	}
	return out, nil
}
