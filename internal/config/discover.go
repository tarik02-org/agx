package config

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v4"
)

// FoundSkill is a directory holding SKILL.md, found below a package or source root.
type FoundSkill struct {
	ID ID
	// Path is relative to the root, /-separated; "." for a skill at the root.
	Path        string
	Name        string
	Description string
}

// FindSkills lists the skill directories below root in lexical order. A skill's ID is its folder
// name, or for a skill at the root its SKILL.md name. Skills are not searched inside other skills,
// and hidden directories and node_modules are skipped. Folders whose names cannot be IDs are
// skipped too.
func FindSkills(root string) ([]FoundSkill, error) {
	found := []FoundSkill{}
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if current != root && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules") {
			return filepath.SkipDir
		}
		document, err := os.ReadFile(filepath.Join(current, "SKILL.md"))
		if os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		skill := FoundSkill{Path: filepath.ToSlash(rel)}
		skill.Name, skill.Description = FrontMatter(document)
		name := path.Base(skill.Path)
		if skill.Path == "." {
			name = skill.Name
		}
		if validIDSegment.MatchString(name) {
			skill.ID = ID(name)
			found = append(found, skill)
		}
		return filepath.SkipDir
	})
	if err != nil {
		return nil, fmt.Errorf("finding skills in %s: %w", root, err)
	}
	return found, nil
}

// FrontMatter reads name and description from SKILL.md front matter; malformed front matter
// yields empty values, as the harness reports it at launch.
func FrontMatter(document []byte) (string, string) {
	scanner := bufio.NewScanner(bytes.NewReader(document))
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return "", ""
	}
	var head bytes.Buffer
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "---" {
			var fields struct {
				Name        string `yaml:"name"`
				Description string `yaml:"description"`
			}
			if yaml.Unmarshal(head.Bytes(), &fields) != nil {
				return "", ""
			}
			return fields.Name, fields.Description
		}
		head.Write(scanner.Bytes())
		head.WriteByte('\n')
	}
	return "", ""
}
