package config

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"

	"github.com/tarik02-org/agx/internal/resources"
)

// ReadContent captures a skill without creating any files. Its source version retains the
// package-lock identity used by snapshots; the returned tree has its own content digest.
func (s Skill) ReadContent() (resources.Tree, string, error) {
	tree := resources.Tree{}
	version, err := s.hashTree(func(rel string, mode fs.FileMode, directory bool) (io.WriteCloser, error) {
		if directory {
			if rel != "." {
				tree.Directories = append(tree.Directories, rel)
			}
			return nil, nil
		}
		buffer := new(bytes.Buffer)
		tree.Files = append(tree.Files, resources.File{Path: rel, Executable: mode&0o111 != 0})
		return &skillContentWriter{Buffer: buffer, file: &tree.Files[len(tree.Files)-1]}, nil
	})
	if s.installed != "" {
		version = s.installed
	}
	return tree, version, err
}

type skillContentWriter struct {
	*bytes.Buffer
	file *resources.File
}

func (w *skillContentWriter) Close() error {
	w.file.Content = w.Bytes()
	return nil
}

// InstructionsContent joins selected instructions in snapshot order and rejects changed sources.
func (s *Snapshot) InstructionsContent() ([]byte, error) {
	var text bytes.Buffer
	for _, r := range s.Resources {
		if r.Kind != "instructions" {
			continue
		}
		b, err := s.Config.Instructions[r.ID].Read()
		if err != nil {
			return nil, err
		}
		if expected, ok := s.InstructionHashes[r.ID]; ok && Digest(string(b)) != expected {
			return nil, fmt.Errorf("instructions %s changed while preparing launch; retry", r.ID)
		}
		text.Write(b)
		text.WriteString("\n\n")
	}
	return text.Bytes(), nil
}

// Content captures the selected resources without deploying them for any harness.
func (s *Snapshot) Content() (resources.Set, error) {
	text, err := s.InstructionsContent()
	if err != nil {
		return resources.Set{}, err
	}
	set := resources.Set{Instructions: string(text)}
	for _, r := range s.Resources {
		if r.Kind != "skill" {
			continue
		}
		skill := s.Config.Skills[r.ID]
		tree, version, err := skill.ReadContent()
		if err != nil {
			return resources.Set{}, fmt.Errorf("skill %s: %w", r.ID, err)
		}
		if version != s.SkillHashes[r.ID] {
			return resources.Set{}, fmt.Errorf("skill %s changed while preparing launch; retry", r.ID)
		}
		var document []byte
		for _, file := range tree.Files {
			if file.Path == "SKILL.md" {
				document = file.Content
				break
			}
		}
		set.Skills = append(set.Skills, resources.Skill{
			ID: string(r.ID), Name: skill.nativeName(document), Digest: tree.Digest(), Content: tree,
		})
	}
	return set, nil
}
