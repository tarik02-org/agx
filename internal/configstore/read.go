package configstore

import (
	"context"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/storage/statedb"
)

func readLocalResources(ctx context.Context, q *statedb.Queries, root string, defs map[string]localDefinition) error {
	skills, err := q.ListLocalSkills(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range skills {
		m := defs[r.Key].body
		readLocalText(m, "path", r.Path)
		readLocalText(m, "git", r.Git)
		readLocalText(m, "content", r.Content)
		if r.Content.Valid {
			m["description"] = r.Description
		}
		localStrings(m, "ref", r.Ref, "hash", r.Hash, "name", r.NativeName)
		if err := readLocalJSON(m, "frontmatter", r.Frontmatter); err != nil {
			return err
		}
	}
	instructions, err := q.ListLocalInstructions(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range instructions {
		m := defs[r.Key].body
		readLocalText(m, "path", r.Path)
		readLocalText(m, "git", r.Git)
		readLocalText(m, "text", r.Text)
		localStrings(m, "ref", r.Ref, "hash", r.Hash)
		if r.SortOrder != 0 {
			m["order"] = r.SortOrder
		}
	}
	packages, err := q.ListLocalPackages(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range packages {
		m := defs[r.Key].body
		readLocalText(m, "path", r.Path)
		readLocalText(m, "git", r.Git)
		localStrings(m, "ref", r.Ref, "hash", r.Hash, "subdir", r.Subdir)
	}
	harnesses, err := q.ListLocalHarnesses(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range harnesses {
		m := defs[r.Key].body
		localStrings(m, "base", r.Base, "executable", r.Executable)
		if r.Hidden.Valid {
			m["hidden"] = r.Hidden.Bool
		}
		if err := readLocalJSON(m, "settings", r.Settings); err != nil {
			return err
		}
	}
	secrets, err := q.ListLocalSecrets(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range secrets {
		m := defs[r.Key].body
		readLocalText(m, "file", r.File)
		localStrings(
			m,
			"kind",
			r.SecretKind,
			"issuer",
			r.Issuer,
			"client_id",
			r.ClientID,
			"resource",
			r.Resource,
		)
		if r.AgeKeyKind != "literal" || r.AgeKeyValue != "" {
			m["age_key"] = storedLocalValue(r.AgeKeyKind, r.AgeKeyValue)
		}
	}
	servers, err := q.ListLocalMCPs(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range servers {
		m := defs[r.Key].body
		localStrings(m, "url", r.Url, "cwd", r.Cwd, "description", r.Description, "load", r.Load)
		if r.ShareIdle.Valid {
			m["share"] = map[string]any{"type": r.ShareType, "idle": r.ShareIdle.String}
		} else if r.ShareType != "" {
			m["share"] = r.ShareType
		}
		switch r.AuthKind {
		case "default":
		case "inline":
			m["auth"] = defs[r.InlineSecret.String].body
		case "none":
			m["auth"] = "none"
		default:
			m["auth"] = storedLocalValue(r.AuthKind, r.AuthValue)
		}
	}
	return nil
}

func readLocalChildren(ctx context.Context, q *statedb.Queries, root string, defs map[string]localDefinition) error {
	files, err := q.ListLocalSkillFiles(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range files {
		file := map[string]any{}
		if r.Path != "" {
			file["path"] = r.Path
		} else {
			file["content"] = r.Content
		}
		if r.Executable {
			file["executable"] = true
		}
		localMap(defs[r.Key].body, "files")[r.Name] = file
	}
	commands, err := q.ListLocalCommands(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range commands {
		localAppend(defs[r.Key].body, "command", r.Value)
	}
	env, err := q.ListLocalEnvironments(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range env {
		localMap(defs[r.Key].body, "environment")[r.Name] = storedLocalValue(r.ValueKind, r.Value)
	}
	headers, err := q.ListLocalHeaders(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range headers {
		localMap(defs[r.Key].body, "headers")[r.Name] = storedLocalValue(r.ValueKind, r.Value)
	}
	groups, err := q.ListLocalArgGroups(ctx, root)
	if err != nil {
		return err
	}
	args := map[localGroup]map[string]any{}
	for _, r := range groups {
		group := map[string]any{"values": []any{}}
		args[localGroup{r.Key, r.Position}] = group
		localAppend(defs[r.Key].body, "args", group)
	}
	values, err := q.ListLocalArguments(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range values {
		localAppend(args[localGroup{r.Key, r.GroupPosition}], "values", storedLocalValue(r.ValueKind, r.Value))
	}
	modes, err := q.ListLocalArgumentModes(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range modes {
		localAppend(args[localGroup{r.Key, r.GroupPosition}], "modes", r.Mode)
	}
	keys, err := q.ListLocalSecretKeys(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range keys {
		localAppend(defs[r.Key].body, "key", r.Value)
	}
	scopes, err := q.ListLocalSecretScopes(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range scopes {
		localAppend(defs[r.Key].body, "scopes", r.Value)
	}
	return nil
}

func readLocalSelections(
	ctx context.Context,
	q *statedb.Queries,
	root string,
	out *config.Local,
	defs map[string]localDefinition,
) error {
	rows, err := q.ListLocalScopes(ctx, root)
	if err != nil {
		return err
	}
	scopes := map[int64]map[string]any{}
	for _, r := range rows {
		m := map[string]any{}
		localStrings(m, "directory", r.Directory)
		scopes[r.Position] = m
		localAppend(out.Document, "scopes", m)
	}
	harnesses, err := q.ListLocalScopeHarnesses(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range harnesses {
		localAppend(scopes[r.ScopePosition], "harnesses", r.Name)
	}
	selections, err := q.ListLocalDefinitionItems(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range selections {
		node := defs[r.Key]
		target := node.body
		if node.kind != config.KindBundles {
			target = localMap(target, r.Action)
		}
		var value any = r.Reference.String
		if r.InlineKey.Valid {
			value = defs[r.InlineKey.String].body
		}
		localAppend(target, r.Kind, value)
	}
	items, err := q.ListLocalScopeItems(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range items {
		var value any = r.Reference.String
		if r.InlineKey.Valid {
			value = defs[r.InlineKey.String].body
		}
		localAppend(localMap(scopes[r.ScopePosition], r.Action), r.Kind, value)
	}
	return nil
}
