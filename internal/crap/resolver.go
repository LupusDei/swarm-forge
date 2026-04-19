package crap

import (
	"os"
	"path/filepath"
	"strings"
)

// pathResolver maps on-disk Go source paths to the package-qualified
// paths that appear in coverprofiles, by reading the nearest go.mod.
type pathResolver struct {
	moduleRoot string
	modulePath string
}

func newResolver(root string) (*pathResolver, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	modRoot, modPath := findModule(abs)
	return &pathResolver{moduleRoot: modRoot, modulePath: modPath}, nil
}

func findModule(start string) (string, string) {
	dir := start
	for {
		path, ok := readModulePath(dir)
		if ok {
			return dir, path
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}

func readModulePath(dir string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", false
	}
	return parseModulePath(data)
}

func parseModulePath(data []byte) (string, bool) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "module ") {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(line, "module ")), true
	}
	return "", false
}

func (r *pathResolver) blockPath(onDisk string) string {
	if r.empty() {
		return onDisk
	}
	return r.qualify(onDisk)
}

func (r *pathResolver) empty() bool {
	return r == nil || r.moduleRoot == ""
}

func (r *pathResolver) qualify(onDisk string) string {
	abs, err := filepath.Abs(onDisk)
	if err != nil {
		return onDisk
	}
	rel, err := filepath.Rel(r.moduleRoot, abs)
	if err != nil {
		return onDisk
	}
	return r.modulePath + "/" + filepath.ToSlash(rel)
}
