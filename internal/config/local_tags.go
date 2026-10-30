package config

import (
	"fmt"
	"slices"

	"go.yaml.in/yaml/v4"
)

func localField(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func localTags(n *yaml.Node) error {
	if n.Kind == yaml.DocumentNode {
		n = n.Content[0]
	}
	for _, kind := range Kinds {
		definitions := localField(n, string(kind))
		if definitions == nil || definitions.Kind != yaml.MappingNode {
			continue
		}
		for i := 1; i < len(definitions.Content); i += 2 {
			if err := localDefinitionTags(kind, definitions.Content[i]); err != nil {
				return err
			}
		}
	}
	if scopes := localField(n, "scopes"); scopes != nil {
		for _, scope := range scopes.Content {
			for _, action := range []string{"add", "remove"} {
				if err := localSelectionTags(localField(scope, action)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func localDefinitionTags(kind Kind, n *yaml.Node) error {
	switch kind {
	case KindMCPServers, KindHarnesses:
		for _, field := range []string{"environment", "headers"} {
			if values := localField(n, field); values != nil && values.Kind == yaml.MappingNode {
				for i := 1; i < len(values.Content); i += 2 {
					if err := localValueTag(values.Content[i], "$env", "$file", "$secret"); err != nil {
						return err
					}
				}
			}
		}
		if kind == KindMCPServers {
			auth := localField(n, "auth")
			if localField(auth, "kind") != nil {
				return localDefinitionTags(KindSecrets, auth)
			}
			return localValueTag(auth, "$env", "$file", "$secret")
		}
		if args := localField(n, "args"); args != nil {
			for _, arg := range args.Content {
				if values := localField(arg, "values"); values != nil {
					for _, value := range values.Content {
						if err := localValueTag(value, "$env", "$file", "$secret"); err != nil {
							return err
						}
					}
				} else if err := localValueTag(arg, "$env", "$file", "$secret"); err != nil {
					return err
				}
			}
		}
		if err := localSettingsTags(localField(n, "settings")); err != nil {
			return err
		}
		for _, action := range []string{"add", "remove"} {
			if err := localSelectionTags(localField(n, action)); err != nil {
				return err
			}
		}
	case KindSecrets:
		return localValueTag(localField(n, "age_key"), "$env", "$file", "$secret")
	case KindBundles:
		return localSelectionTags(n)
	case KindSkills, KindInstructions, KindPackages:
	}
	return nil
}

func localSelectionTags(n *yaml.Node) error {
	for _, kind := range SelectionKinds {
		if entries := localField(n, string(kind)); entries != nil && entries.Kind == yaml.SequenceNode {
			for _, entry := range entries.Content {
				if entry.Kind == yaml.MappingNode {
					if err := localDefinitionTags(kind, entry); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func localValueTag(n *yaml.Node, allowed ...string) error {
	if n == nil || n.Kind != yaml.MappingNode || len(n.Content) != 2 {
		return nil
	}
	key, value := n.Content[0].Value, n.Content[1]
	if !slices.Contains(allowed, key) {
		return nil
	}
	if value.Tag != "!!str" {
		return fmt.Errorf("%s needs a string", key)
	}
	*n = *value
	n.Tag = "!" + key[1:]
	return nil
}

func localSettingsTags(n *yaml.Node) error {
	if n == nil {
		return nil
	}
	// Native settings allow file injection only; other marker-shaped objects are literal JSON.
	if err := localValueTag(n, "$file"); err != nil {
		return err
	}
	if n.Tag == "!file" {
		return nil
	}
	for _, child := range n.Content {
		if err := localSettingsTags(child); err != nil {
			return err
		}
	}
	return nil
}
