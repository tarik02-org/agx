// Command schemagen writes the configuration JSON Schemas published in the repository.
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/config"
)

//go:generate go run . ../../../schema

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: schemagen <directory>")
	}
	for name, manifest := range map[string]bool{"agx.schema.json": false, "agx.package.schema.json": true} {
		b, err := config.Schema(manifest, builtin.Registry().Builtins(nil))
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(os.Args[1], name), b, 0o644); err != nil {
			log.Fatal(err)
		}
	}
}
