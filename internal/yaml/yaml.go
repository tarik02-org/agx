// Package yaml is agx's only import of a YAML library. Configuration editing needs a node tree
// with positions, comments, and styles, so the rest of agx uses these names; replacing the
// library, such as when go.yaml.in/yaml/v4 leaves its release candidates, changes only this file.
package yaml

import (
	"io"

	yaml "go.yaml.in/yaml/v4"
)

type (
	Node       = yaml.Node
	Kind       = yaml.Kind
	Style      = yaml.Style
	Option     = yaml.Option
	Decoder    = yaml.Decoder
	LoadError  = yaml.LoadError
	LoadErrors = yaml.LoadErrors
)

const (
	DocumentNode = yaml.DocumentNode
	SequenceNode = yaml.SequenceNode
	MappingNode  = yaml.MappingNode
	ScalarNode   = yaml.ScalarNode
	AliasNode    = yaml.AliasNode

	FlowStyle    = yaml.FlowStyle
	LiteralStyle = yaml.LiteralStyle
)

func Unmarshal(in []byte, out any) error { return yaml.Unmarshal(in, out) }

func Marshal(in any) ([]byte, error) { return yaml.Marshal(in) }

func Dump(in any, options ...Option) ([]byte, error) { return yaml.Dump(in, options...) }

func NewDecoder(r io.Reader) *Decoder { return yaml.NewDecoder(r) }

func WithKnownFields() Option { return yaml.WithKnownFields() }

func WithV4Defaults() Option { return yaml.WithV4Defaults() }

func WithIndent(indent int) Option { return yaml.WithIndent(indent) }

func WithCompactSeqIndent(compact bool) Option { return yaml.WithCompactSeqIndent(compact) }

func WithLineWidth(width int) Option { return yaml.WithLineWidth(width) }
