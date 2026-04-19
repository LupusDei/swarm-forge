package start_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/swarm-forge/swarm-forge/internal/start"
)

// noopSleeper satisfies tmux.Sleeper without any wall-clock delay.
type noopSleeper struct{}

func (noopSleeper) Sleep(_ time.Duration) {}

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

// Attach satisfies the forthcoming Commander.Attach method.
// Recording it in calls lets tests assert Attach is the final action.
func (r *recCmd) Attach(session string) error {
	r.calls = append(r.calls, []string{"attach-session", "-t", session})
	return nil
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
		Sleeper:          noopSleeper{},
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

// C5 red — the metrics pane init goes through tmux.SendKeys, which now splits
// delivery into TWO calls (text with -l, then a standalone Enter). Both calls
// must target pane 3, and they must both land BEFORE the final Attach.
func TestRunInitsMetricsPaneAsTwoSendKeysCalls(t *testing.T) {
	cmd := newRecCmd()
	fs := newFakeFS()
	fs.files["/project/Constitution.md"] = []byte("constitution")
	var stdout bytes.Buffer

	if err := start.Run(fullCfg(cmd, fs, &stdout)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Locate the two metrics-pane send-keys invocations: they target pane 3.
	const pane3 = "swarmforge:swarm.3"
	var metricsCalls [][]string
	var metricsIdxs []int
	for i, c := range cmd.calls {
		if len(c) == 0 || c[0] != "send-keys" {
			continue
		}
		joined := strings.Join(c, " ")
		if strings.Contains(joined, pane3) {
			metricsCalls = append(metricsCalls, c)
			metricsIdxs = append(metricsIdxs, i)
		}
	}
	if len(metricsCalls) != 2 {
		t.Fatalf("metrics pane init must produce exactly 2 send-keys calls to %s; got %d: %v",
			pane3, len(metricsCalls), metricsCalls)
	}

	first := metricsCalls[0]
	if !strings.Contains(strings.Join(first, " "), "tail -f logs/agent_messages.log") {
		t.Errorf("first metrics call must carry the tail command; got: %v", first)
	}
	firstHasLiteral := false
	for _, a := range first {
		if a == "-l" {
			firstHasLiteral = true
			break
		}
	}
	if !firstHasLiteral {
		t.Errorf("first metrics call must use '-l' literal flag; got: %v", first)
	}

	second := metricsCalls[1]
	if len(second) == 0 || second[len(second)-1] != "Enter" {
		t.Errorf("second metrics call must end with standalone 'Enter'; got: %v", second)
	}
	for _, a := range second {
		if strings.Contains(a, "tail -f") {
			t.Errorf("second metrics call must NOT repeat the tail payload; got: %v", second)
		}
	}

	// Attach must come AFTER both metrics calls.
	attachIdx := lastIndexOfCall(cmd.calls, "attach-session")
	if attachIdx < 0 {
		t.Fatalf("attach-session not found; calls=%v", cmd.calls)
	}
	for _, mi := range metricsIdxs {
		if attachIdx <= mi {
			t.Errorf("attach must follow every metrics init call; attachIdx=%d metricsIdx=%d", attachIdx, mi)
		}
	}
}

// lastIndexOfCall returns the index of the last call whose args contain keyword, or -1.
func lastIndexOfCall(calls [][]string, keyword string) int {
	idx := -1
	for i, c := range calls {
		for _, a := range c {
			if strings.Contains(a, keyword) {
				idx = i
				break
			}
		}
	}
	return idx
}

// C3 red test — Attach must be called with the session name and appear as the final action.
func TestRunAttachesToSessionAsFinalStep(t *testing.T) {
	cmd := newRecCmd()
	fs := newFakeFS()
	fs.files["/project/Constitution.md"] = []byte("constitution")
	var stdout bytes.Buffer

	err := start.Run(fullCfg(cmd, fs, &stdout))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	attachIdx := lastIndexOfCall(cmd.calls, "attach-session")
	if attachIdx < 0 {
		t.Fatalf("start.Run must attach to the session; calls=%v", cmd.calls)
	}
	metricsIdx := lastIndexOfCall(cmd.calls, "tail -f logs/agent_messages.log")
	if metricsIdx < 0 {
		t.Fatalf("metrics pane init missing; calls=%v", cmd.calls)
	}
	if attachIdx <= metricsIdx {
		t.Fatalf("attach must come after metrics pane init; attachIdx=%d metricsIdx=%d calls=%v",
			attachIdx, metricsIdx, cmd.calls)
	}
	if attachIdx != len(cmd.calls)-1 {
		t.Fatalf("attach must be the final call; attachIdx=%d total=%d calls=%v",
			attachIdx, len(cmd.calls), cmd.calls)
	}

	// Also verify the session name was passed through.
	attachCall := cmd.calls[attachIdx]
	joined := strings.Join(attachCall, " ")
	if !strings.Contains(joined, "swarmforge") {
		t.Fatalf("attach call must target session 'swarmforge'; got: %v", attachCall)
	}
}

// C3 red test — Attach must NOT be called when preflight fails fast.
func TestRunDoesNotAttachOnPreflightFailure(t *testing.T) {
	cmd := newRecCmd()
	fs := newFakeFS()
	fs.files["/project/Constitution.md"] = []byte("constitution")
	var stdout bytes.Buffer

	cfg := fullCfg(cmd, fs, &stdout)
	cfg.LookPath = func(name string) (string, error) {
		return "", errors.New(name + ": not found")
	}
	if err := start.Run(cfg); err == nil {
		t.Fatal("expected preflight error")
	}
	if hasCall(cmd.calls, "attach-session") {
		t.Fatalf("attach must not be called when preflight fails; calls=%v", cmd.calls)
	}
}
