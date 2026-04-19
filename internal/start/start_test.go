package start_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/swarm-forge/swarm-forge/internal/start"
)

type recCmd struct {
	calls    [][]string
	sessions map[string]bool
}

func newRecCmd() *recCmd {
	return &recCmd{sessions: make(map[string]bool)}
}

func (r *recCmd) Run(args ...string) error {
	r.calls = append(r.calls, args)
	return nil
}

func (r *recCmd) HasSession(name string) bool {
	return r.sessions[name]
}

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

func passingLookPath(name string) (string, error) {
	return "/usr/bin/" + name, nil
}

func fullCfg(cmd *recCmd, fs *fakeFS, stdout *bytes.Buffer) start.Config {
	return start.Config{
		Commander:        cmd,
		Session:          "swarmforge",
		ProjectRoot:      "/project",
		FS:               fs,
		LookPath:         passingLookPath,
		ConstitutionPath: "Constitution.md",
		Stdout:           stdout,
	}
}

func hasCall(calls [][]string, keyword string) bool {
	for _, c := range calls {
		for _, a := range c {
			if strings.Contains(a, keyword) {
				return true
			}
		}
	}
	return false
}

func TestRunFullSequence(t *testing.T) {
	cmd := newRecCmd()
	fs := newFakeFS()
	fs.files["/project/Constitution.md"] = []byte("Rule 1: TDD")
	var stdout bytes.Buffer

	err := start.Run(fullCfg(cmd, fs, &stdout))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasCall(cmd.calls, "new-session") {
		t.Fatal("should create session")
	}
	if !strings.Contains(stdout.String(), "SwarmForge") {
		t.Fatal("should print banner")
	}
}

func TestRunKillsExistingSession(t *testing.T) {
	cmd := newRecCmd()
	cmd.sessions["swarmforge"] = true
	fs := newFakeFS()
	fs.files["/project/Constitution.md"] = []byte("constitution")
	var stdout bytes.Buffer

	err := start.Run(fullCfg(cmd, fs, &stdout))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasCall(cmd.calls, "kill-session") {
		t.Fatal("should kill existing session")
	}
}

func TestRunNoExistingSession(t *testing.T) {
	cmd := newRecCmd()
	fs := newFakeFS()
	fs.files["/project/Constitution.md"] = []byte("constitution")
	var stdout bytes.Buffer

	err := start.Run(fullCfg(cmd, fs, &stdout))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasCall(cmd.calls, "kill-session") {
		t.Fatal("should not kill when no session")
	}
}

func TestRunFailsOnMissingDep(t *testing.T) {
	cmd := newRecCmd()
	fs := newFakeFS()
	fs.files["/project/Constitution.md"] = []byte("constitution")
	var stdout bytes.Buffer

	cfg := fullCfg(cmd, fs, &stdout)
	cfg.LookPath = func(name string) (string, error) {
		return "", errors.New(name + ": not found")
	}
	err := start.Run(cfg)
	if err == nil {
		t.Fatal("expected preflight error")
	}
	if len(cmd.calls) > 0 {
		t.Fatal("no tmux calls after preflight failure")
	}
}

func TestRunFailsOnMissingConstitution(t *testing.T) {
	cmd := newRecCmd()
	fs := newFakeFS() // no constitution
	var stdout bytes.Buffer

	err := start.Run(fullCfg(cmd, fs, &stdout))
	if err == nil {
		t.Fatal("expected constitution error")
	}
	if len(cmd.calls) > 0 {
		t.Fatal("no tmux calls after constitution error")
	}
}

func TestRunWritesPromptFiles(t *testing.T) {
	cmd := newRecCmd()
	fs := newFakeFS()
	fs.files["/project/Constitution.md"] = []byte("Rule 1: TDD")
	var stdout bytes.Buffer

	err := start.Run(fullCfg(cmd, fs, &stdout))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, name := range []string{"Architect", "Coder", "E2E-Interpreter"} {
		path := "/project/.swarmforge/prompts/" + name + ".md"
		data, ok := fs.files[path]
		if !ok {
			t.Fatalf("prompt not written for %s", name)
		}
		if !strings.Contains(string(data), "Rule 1: TDD") {
			t.Fatalf("prompt for %s missing constitution", name)
		}
	}
}

func TestRunLaunchesAgents(t *testing.T) {
	cmd := newRecCmd()
	fs := newFakeFS()
	fs.files["/project/Constitution.md"] = []byte("constitution")
	var stdout bytes.Buffer

	err := start.Run(fullCfg(cmd, fs, &stdout))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasCall(cmd.calls, "SwarmForge Architect") {
		t.Fatal("missing Architect launch")
	}
	if !hasCall(cmd.calls, "SwarmForge Coder") {
		t.Fatal("missing Coder launch")
	}
	if !hasCall(cmd.calls, "SwarmForge E2E-Interpreter") {
		t.Fatal("missing E2E-Interpreter launch")
	}
}

func TestRunInitsMetricsPane(t *testing.T) {
	cmd := newRecCmd()
	fs := newFakeFS()
	fs.files["/project/Constitution.md"] = []byte("constitution")
	var stdout bytes.Buffer

	err := start.Run(fullCfg(cmd, fs, &stdout))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasCall(cmd.calls, "tail -f logs/agent_messages.log") {
		t.Fatal("missing metrics pane tail command")
	}
}
