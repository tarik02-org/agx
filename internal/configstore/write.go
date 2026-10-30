package configstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/storage/statedb"
)

func writeLocalDefinition(
	ctx context.Context,
	q *statedb.Queries,
	root, key string,
	kind config.Kind,
	name string,
	parent sql.NullString,
	scope sql.NullInt64,
	m map[string]any,
) error {
	if err := q.InsertLocalDefinition(
		ctx,
		statedb.InsertLocalDefinitionParams{
			Root:     root,
			Key:      key,
			Kind:     string(kind),
			Name:     name,
			Parent:   parent,
			Scope:    scope,
			Override: localFlag(m, "override"),
		},
	); err != nil {
		return err
	}
	switch kind {
	case config.KindSkills:
		frontmatter, err := localJSON(m["frontmatter"])
		if err != nil {
			return err
		}
		if err := q.InsertLocalSkill(
			ctx,
			statedb.InsertLocalSkillParams{
				Root:        root,
				Key:         key,
				Path:        localOptionalText(m, "path"),
				Git:         localOptionalText(m, "git"),
				Ref:         localText(m, "ref"),
				Hash:        localText(m, "hash"),
				NativeName:  localText(m, "name"),
				Description: localText(m, "description"),
				Content:     localOptionalText(m, "content"),
				Frontmatter: frontmatter,
			},
		); err != nil {
			return err
		}
		for name, value := range localObject(m["files"]) {
			file := localObject(value)
			if err := q.InsertLocalSkillFile(
				ctx,
				statedb.InsertLocalSkillFileParams{
					Root:       root,
					Key:        key,
					Name:       name,
					Content:    localText(file, "content"),
					Path:       localText(file, "path"),
					Executable: localFlag(file, "executable"),
				},
			); err != nil {
				return err
			}
		}
	case config.KindInstructions:
		if err := q.InsertLocalInstruction(
			ctx,
			statedb.InsertLocalInstructionParams{
				Root:      root,
				Key:       key,
				Path:      localOptionalText(m, "path"),
				Git:       localOptionalText(m, "git"),
				Ref:       localText(m, "ref"),
				Hash:      localText(m, "hash"),
				Text:      localOptionalText(m, "text"),
				SortOrder: localInteger(m["order"]),
			},
		); err != nil {
			return err
		}
	case config.KindPackages:
		if err := q.InsertLocalPackage(
			ctx,
			statedb.InsertLocalPackageParams{
				Root:   root,
				Key:    key,
				Path:   localOptionalText(m, "path"),
				Git:    localOptionalText(m, "git"),
				Ref:    localText(m, "ref"),
				Hash:   localText(m, "hash"),
				Subdir: localText(m, "subdir"),
			},
		); err != nil {
			return err
		}
	case config.KindHarnesses:
		hidden, present := m["hidden"].(bool)
		settings, err := localJSON(m["settings"])
		if err != nil {
			return err
		}
		if err := q.InsertLocalHarness(
			ctx,
			statedb.InsertLocalHarnessParams{
				Root:       root,
				Key:        key,
				Base:       localText(m, "base"),
				Executable: localText(m, "executable"),
				Hidden:     sql.NullBool{Bool: hidden, Valid: present},
				Settings:   settings,
			},
		); err != nil {
			return err
		}
		if err := writeLocalArgs(ctx, q, root, key, localArray(m["args"])); err != nil {
			return err
		}
		for _, action := range []string{"add", "remove"} {
			if err := writeLocalSelection(
				ctx,
				q,
				root,
				key,
				sql.NullInt64{},
				action,
				localObject(m[action]),
			); err != nil {
				return err
			}
		}
	case config.KindMCPServers:
		if err := writeLocalMCP(ctx, q, root, key, m); err != nil {
			return err
		}
	case config.KindSecrets:
		ageKind, ageValue := "literal", ""
		if value, found := m["age_key"]; found {
			var err error
			ageKind, ageValue, err = localValue(value)
			if err != nil {
				return err
			}
		}
		if err := q.InsertLocalSecret(
			ctx,
			statedb.InsertLocalSecretParams{
				Root:        root,
				Key:         key,
				SecretKind:  localText(m, "kind"),
				File:        localOptionalText(m, "file"),
				AgeKeyKind:  ageKind,
				AgeKeyValue: ageValue,
				Issuer:      localText(m, "issuer"),
				ClientID:    localText(m, "client_id"),
				Resource:    localText(m, "resource"),
			},
		); err != nil {
			return err
		}
		keys := localArray(m["key"])
		if dotted, ok := m["key"].(string); ok {
			for _, segment := range strings.Split(dotted, ".") {
				keys = append(keys, segment)
			}
		}
		for i, value := range keys {
			if err := q.InsertLocalSecretKey(
				ctx,
				statedb.InsertLocalSecretKeyParams{Root: root, Key: key, Position: int64(i), Value: value.(string)},
			); err != nil {
				return err
			}
		}
		for i, value := range localArray(m["scopes"]) {
			if err := q.InsertLocalSecretScope(
				ctx,
				statedb.InsertLocalSecretScopeParams{Root: root, Key: key, Position: int64(i), Value: value.(string)},
			); err != nil {
				return err
			}
		}
	case config.KindBundles:
		if err := writeLocalSelection(ctx, q, root, key, sql.NullInt64{}, "add", m); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown local definition kind %q", kind)
	}
	return writeLocalValues(ctx, q, root, key, m)
}

func writeLocalMCP(ctx context.Context, q *statedb.Queries, root, key string, m map[string]any) error {
	authKind, authValue := "default", ""
	var inline sql.NullString
	if auth, found := m["auth"]; found {
		secret := localObject(auth)
		switch auth {
		case "none":
			authKind = "none"
		case "keychain", "oauth":
			secret = map[string]any{"kind": auth}
		}
		if localText(secret, "kind") != "" {
			authKind = "inline"
			inline = sql.NullString{String: key + "/auth", Valid: true}
			if err := writeLocalDefinition(
				ctx,
				q,
				root,
				inline.String,
				config.KindSecrets,
				"",
				sql.NullString{String: key, Valid: true},
				sql.NullInt64{},
				secret,
			); err != nil {
				return err
			}
		} else if authKind == "default" {
			var err error
			authKind, authValue, err = localValue(auth)
			if err != nil {
				return err
			}
		}
	}
	shareType := ""
	var idle sql.NullString
	if share, ok := m["share"].(string); ok {
		shareType = share
	} else if share := localObject(m["share"]); share != nil {
		shareType = localText(share, "type")
		if value, ok := share["idle"].(string); ok {
			idle = sql.NullString{String: value, Valid: true}
		}
	}
	return q.InsertLocalMCP(
		ctx,
		statedb.InsertLocalMCPParams{
			Root:         root,
			Key:          key,
			Url:          localText(m, "url"),
			Cwd:          localText(m, "cwd"),
			Description:  localText(m, "description"),
			Load:         localText(m, "load"),
			ShareType:    shareType,
			ShareIdle:    idle,
			AuthKind:     authKind,
			AuthValue:    authValue,
			InlineSecret: inline,
		},
	)
}

func writeLocalValues(ctx context.Context, q *statedb.Queries, root, key string, m map[string]any) error {
	for i, value := range localArray(m["command"]) {
		if err := q.InsertLocalCommand(
			ctx,
			statedb.InsertLocalCommandParams{Root: root, Key: key, Position: int64(i), Value: value.(string)},
		); err != nil {
			return err
		}
	}
	for name, value := range localObject(m["environment"]) {
		kind, text, err := localValue(value)
		if err != nil {
			return err
		}
		if err := q.InsertLocalEnvironment(
			ctx,
			statedb.InsertLocalEnvironmentParams{Root: root, Key: key, Name: name, ValueKind: kind, Value: text},
		); err != nil {
			return err
		}
	}
	for name, value := range localObject(m["headers"]) {
		kind, text, err := localValue(value)
		if err != nil {
			return err
		}
		if err := q.InsertLocalHeader(
			ctx,
			statedb.InsertLocalHeaderParams{Root: root, Key: key, Name: name, ValueKind: kind, Value: text},
		); err != nil {
			return err
		}
	}
	return nil
}
