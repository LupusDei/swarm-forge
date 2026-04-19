package setup_test

import (
	"fmt"
	"testing"

	"github.com/swarm-forge/swarm-forge/internal/setup"
)

type fakeFS struct {
	dirs  []string
	files map[string][]byte
}

func newFakeFS() *fakeFS {
	return &fakeFS{files: make(map[string][]byte)}
}

func (f *fakeFS) MkdirAll(path string, _ uint32) error {
	f.dirs = append(f.dirs, path)
	return nil
}

func (f *fakeFS) WriteFile(path string, data []byte, _ uint32) error {
	f.files[path] = data
	return nil
}

func (f *fakeFS) ReadFile(path string) ([]byte, error) {
	data, ok := f.files[path]
	if !ok {
		return nil, fmt.Errorf("not found: %s", path)
	}
	return data, nil
}

func (f *fakeFS) Stat(path string) (bool, error) {
	_, ok := f.files[path]
	return ok, nil
}

func TestEnsureDirsCreatesAll(t *testing.T) {
	fs := newFakeFS()
	err := setup.EnsureDirs(fs, "/root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"/root/features", "/root/logs", "/root/agent_context"}
	for _, e := range expected {
		found := false
		for _, d := range fs.dirs {
			if d == e {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing dir %q; got %v", e, fs.dirs)
		}
	}
}
