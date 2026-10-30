package config

import (
	"encoding/json"
	"errors"

	"go.yaml.in/yaml/v4"
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
