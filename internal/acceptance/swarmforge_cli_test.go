package acceptance

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/swarm-forge/swarm-forge/internal/banner"
	"github.com/swarm-forge/swarm-forge/internal/cli"
	"github.com/swarm-forge/swarm-forge/internal/notify"
	"github.com/swarm-forge/swarm-forge/internal/preflight"
	"github.com/swarm-forge/swarm-forge/internal/prompt"
	"github.com/swarm-forge/swarm-forge/internal/setup"
	"github.com/swarm-forge/swarm-forge/internal/start"
	"github.com/swarm-forge/swarm-forge/internal/swarmlog"
	"github.com/swarm-forge/swarm-forge/internal/tmux"
)

// ── Recording stubs ─────────────────────────────────────────────────

// RecordingCommander records all tmux commands for verification.
type RecordingCommander struct {
	Calls    [][]string
	Sessions map[string]bool
}

func NewRecordingCommander() *RecordingCommander {
	return &RecordingCommander{Sessions: make(map[string]bool)}
}

func (r *RecordingCommander) Run(args ...string) error {
	r.Calls = append(r.Calls, args)
	return nil
}

func (r *RecordingCommander) HasSession(name string) bool {
	return r.Sessions[name]
}

// FakeFS records filesystem operations for verification.
type FakeFS struct {
	Dirs  []string
	Files map[string][]byte
}

func NewFakeFS() *FakeFS {
	return &FakeFS{Files: make(map[string][]byte)}
}

func (f *FakeFS) MkdirAll(path string, _ uint32) error {
	f.Dirs = append(f.Dirs, path)
	return nil
}

func (f *FakeFS) WriteFile(path string, data []byte, _ uint32) error {
	f.Files[path] = data
	return nil
}

func (f *FakeFS) ReadFile(path string) ([]byte, error) {
	data, ok := f.Files[path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return data, nil
}

func (f *FakeFS) Stat(path string) (bool, error) {
	_, ok := f.Files[path]
	if ok {
		return true, nil
	}
	for _, d := range f.Dirs {
		if d == path {
			return true, nil
		}
	}
	return false, nil
}

// ── Scenario 1: Preflight rejects missing dependency ────────────────

func TestCLI_PreflightRejectsMissingDependency(t *testing.T) {
	// Given the system does not have "tmux" installed
	lookPath := func(name string) (string, error) {
		return "", errors.New(name + ": not found")
	}

	// When the user runs preflight checks
	err := preflight.Check(lookPath, "tmux", "claude", "watch")

	// Then an error is returned containing "tmux"
	if err == nil {
		t.Fatal("expected error for missing tmux, got nil")
	}
	if !strings.Contains(err.Error(), "tmux") {
		t.Fatalf("error should mention tmux, got: %s", err.Error())
	}
}

// ── Scenario 2: Preflight passes with all dependencies ──────────────

func TestCLI_PreflightPassesAllDeps(t *testing.T) {
	// Given the system has "tmux", "claude", and "watch" installed
	lookPath := func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}

	// When the user runs preflight checks
	err := preflight.Check(lookPath, "tmux", "claude", "watch")

	// Then no error is returned
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// ── Scenario 3: Directory setup creates required directories ────────

func TestCLI_DirectorySetupCreatesRequiredDirs(t *testing.T) {
	// Given a project root directory exists
	fs := NewFakeFS()
	root := "/project"

	// When directory setup runs for the project root
	err := setup.EnsureDirs(fs, root)
	if err != nil {
		t.Fatalf("EnsureDirs error: %v", err)
	}

	// Then the directories exist under the project root
	required := []string{"features", "logs", "agent_context"}
	for _, dir := range required {
		expected := root + "/" + dir
		found := false
		for _, d := range fs.Dirs {
			if d == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("directory %q not created; dirs=%v", expected, fs.Dirs)
		}
	}
}

// ── Scenario 4: Helper scripts generated for backward compat ────────

func TestCLI_HelperScriptsAreGenerated(t *testing.T) {
	// Given a project root directory exists
	fs := NewFakeFS()
	root := "/project"

	// When setup writes helper scripts to the project root
	err := setup.WriteHelperScripts(fs, root)
	if err != nil {
		t.Fatalf("WriteHelperScripts error: %v", err)
	}

	// Then "notify-agent.sh" and "swarm-log.sh" exist
	for _, name := range []string{"notify-agent.sh", "swarm-log.sh"} {
		path := root + "/" + name
		if _, ok := fs.Files[path]; !ok {
			t.Fatalf("%q was not written; files=%v", path, fileKeys(fs))
		}
	}
}

// ── Scenario 5: Agent prompt includes role and constitution ─────────

func TestCLI_PromptIncludesRoleAndConstitution(t *testing.T) {
	// Given a constitution with content "Rule 1: TDD is mandatory"
	constitution := "Rule 1: TDD is mandatory"

	// And the agent role is "Architect" with standard instructions
	cfg := prompt.AgentConfig{
		Role:         "Architect",
		Instructions: prompt.ArchitectInstructions,
		Session:      "swarmforge",
		ProjectRoot:  "/project",
	}

	// When the prompt builder generates the prompt
	result := prompt.Build(cfg, constitution)

	// Then the prompt contains expected strings
	assertContains(t, result, "You are the Architect agent")
	assertContains(t, result, "Rule 1: TDD is mandatory")
	assertContains(t, result, "Pane 0 = Architect")
}

// ── Scenario 6: Agent prompt includes coordination instructions ─────

func TestCLI_PromptIncludesCoordinationInstructions(t *testing.T) {
	// Given a constitution with content "Constitution content"
	constitution := "Constitution content"

	// And the agent role is "Coder" with standard instructions
	cfg := prompt.AgentConfig{
		Role:         "Coder",
		Instructions: prompt.CoderInstructions,
		Session:      "swarmforge",
		ProjectRoot:  "/project",
	}

	// When the prompt builder generates the prompt
	result := prompt.Build(cfg, constitution)

	// Then the prompt contains coordination references
	assertContains(t, result, "notify-agent.sh")
	assertContains(t, result, "swarm-log.sh")
	assertContains(t, result, "agent_context/")
}

// ── Scenario 7: Start kills existing session before creating ────────

func TestCLI_StartKillsExistingSession(t *testing.T) {
	// Given a tmux session named "swarmforge" already exists
	cmd := NewRecordingCommander()
	cmd.Sessions["swarmforge"] = true
	fs := NewFakeFS()
	fs.Files["/project/Constitution.md"] = []byte("constitution")
	var stdout bytes.Buffer

	// When the start sequence runs
	cfg := start.Config{
		Commander:        cmd,
		Session:          "swarmforge",
		ProjectRoot:      "/project",
		FS:               fs,
		LookPath:         func(name string) (string, error) { return "/usr/bin/" + name, nil },
		ConstitutionPath: "Constitution.md",
		Stdout:           &stdout,
	}
	err := start.Run(cfg)
	if err != nil {
		t.Fatalf("start.Run error: %v", err)
	}

	// Then the existing "swarmforge" session is killed
	assertCallContains(t, cmd.Calls, "kill-session")

	// And a new "swarmforge" session is created
	assertCallContains(t, cmd.Calls, "new-session")
}

// ── Scenario 8: Start creates tmux session with 2x2 grid layout ────

func TestCLI_StartCreates2x2Grid(t *testing.T) {
	// Given no tmux session named "swarmforge" exists
	cmd := NewRecordingCommander()

	// When the start sequence creates the tmux session
	err := tmux.CreateSession(cmd, "swarmforge", "swarm")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	err = tmux.SplitGrid(cmd, "swarmforge", "swarm")
	if err != nil {
		t.Fatalf("SplitGrid error: %v", err)
	}
	err = tmux.SetPaneTitles(cmd, "swarmforge", "swarm")
	if err != nil {
		t.Fatalf("SetPaneTitles error: %v", err)
	}

	// Then a new tmux session "swarmforge" with window "swarm" is created
	assertCallContains(t, cmd.Calls, "new-session")

	// And the window is split into 4 panes (3 splits)
	splitCount := countCallsContaining(cmd.Calls, "split-window")
	if splitCount != 3 {
		t.Fatalf("expected 3 split-window calls, got %d", splitCount)
	}

	// And pane borders display agent titles
	assertCallContains(t, cmd.Calls, "select-pane")
}

// ── Scenario 9: Agents launched with correct claude commands ────────

func TestCLI_AgentsLaunchedWithClaudeCommands(t *testing.T) {
	// Given a tmux session "swarmforge" with 4 panes exists
	cmd := NewRecordingCommander()
	cmd.Sessions["swarmforge"] = true

	// And agent prompt files have been written
	// When agents are launched in their panes
	agents := []struct {
		pane int
		name string
	}{
		{0, "Architect"},
		{1, "E2E-Interpreter"},
		{2, "Coder"},
	}
	for _, a := range agents {
		promptFile := fmt.Sprintf("/tmp/swarmforge-%s.md", a.name)
		err := tmux.LaunchAgent(cmd, "swarmforge", a.pane, a.name, promptFile, "/project")
		if err != nil {
			t.Fatalf("LaunchAgent(%s) error: %v", a.name, err)
		}
	}

	// Then each pane receives a claude command containing the agent name
	assertSendKeysContains(t, cmd.Calls, "SwarmForge Architect")
	assertSendKeysContains(t, cmd.Calls, "SwarmForge E2E-Interpreter")
	assertSendKeysContains(t, cmd.Calls, "SwarmForge Coder")

	// And each claude command includes "--permission-mode acceptEdits"
	for _, call := range cmd.Calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "claude") {
			assertContains(t, joined, "--permission-mode acceptEdits")
		}
	}
}

// ── Scenario 10: Metrics pane tails the agent log file ──────────────

func TestCLI_MetricsPaneTailsLog(t *testing.T) {
	// Given a tmux session "swarmforge" with 4 panes exists
	cmd := NewRecordingCommander()

	// When the metrics pane is initialized
	err := tmux.SendKeys(cmd, "swarmforge", "swarm", 3, "tail -f logs/agent_messages.log")
	if err != nil {
		t.Fatalf("SendKeys error: %v", err)
	}

	// Then pane 3 receives a command containing "tail -f logs/agent_messages.log"
	found := false
	for _, call := range cmd.Calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "tail -f logs/agent_messages.log") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected tail command in pane 3; calls=%v", cmd.Calls)
	}
}

// ── Scenario 11: Notify logs and sends message to pane ──────────────

func TestCLI_NotifyLogsAndSendsMessage(t *testing.T) {
	// Given a log writer is configured
	var logBuf bytes.Buffer
	logger := swarmlog.New(&logBuf)

	// And a tmux commander is available
	cmd := NewRecordingCommander()
	cmd.Sessions["swarmforge"] = true

	// When the user runs notify for pane 0 with message "hello architect"
	err := notify.Notify(cmd, logger, "swarmforge", 0, "hello architect")
	if err != nil {
		t.Fatalf("Notify error: %v", err)
	}

	// Then a timestamped log entry containing "[pane 0] hello architect" is written
	logOutput := logBuf.String()
	assertContains(t, logOutput, "[pane 0] hello architect")

	// And tmux send-keys is invoked for session "swarmforge" pane 0
	assertCallContains(t, cmd.Calls, "send-keys")
}

// ── Scenario 12: Log subcommand writes timestamped entry ────────────

func TestCLI_LogWritesTimestampedEntry(t *testing.T) {
	// Given a log writer and stdout writer are configured
	var logBuf bytes.Buffer
	var stdBuf bytes.Buffer
	logger := swarmlog.New(&logBuf, &stdBuf)

	// When the user logs a message with role "Architect" and text "task started"
	err := logger.Write("Architect", "task started")
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Then the log writer contains "[Architect] task started"
	assertContains(t, logBuf.String(), "[Architect] task started")

	// And the stdout writer contains "[Architect] task started"
	assertContains(t, stdBuf.String(), "[Architect] task started")
}

// ── Scenario 13: CLI dispatches subcommands correctly ───────────────

func TestCLI_DispatchesSubcommands(t *testing.T) {
	var startCalled, notifyCalled, logCalled bool

	cfg := cli.Config{
		Start:  func(_ []string) error { startCalled = true; return nil },
		Notify: func(_ []string) error { notifyCalled = true; return nil },
		Log:    func(_ []string) error { logCalled = true; return nil },
	}

	// Given the CLI receives arguments "start"
	err := cli.Dispatch([]string{"start"}, cfg)
	if err != nil {
		t.Fatalf("dispatch start error: %v", err)
	}
	// Then the start handler is invoked
	if !startCalled {
		t.Fatal("start handler was not called")
	}

	// Given the CLI receives arguments "notify" "1" "hello"
	err = cli.Dispatch([]string{"notify", "1", "hello"}, cfg)
	if err != nil {
		t.Fatalf("dispatch notify error: %v", err)
	}
	// Then the notify handler is invoked
	if !notifyCalled {
		t.Fatal("notify handler was not called")
	}

	// Given the CLI receives arguments "log" "Coder" "done"
	err = cli.Dispatch([]string{"log", "Coder", "done"}, cfg)
	if err != nil {
		t.Fatalf("dispatch log error: %v", err)
	}
	// Then the log handler is invoked
	if !logCalled {
		t.Fatal("log handler was not called")
	}

	// Given the CLI receives no arguments
	err = cli.Dispatch([]string{}, cfg)
	// Then a usage error is returned
	if err == nil {
		t.Fatal("expected usage error for empty args, got nil")
	}
}

// ── Scenario 14: Full startup banner is displayed ───────────────────

func TestCLI_StartupBannerDisplayed(t *testing.T) {
	// Given a writer captures output
	var buf bytes.Buffer

	// When the startup banner is printed
	banner.Print(&buf)

	// Then the output contains "SwarmForge"
	output := buf.String()
	assertContains(t, output, "SwarmForge")

	// And the output contains "Disciplined agents build better software"
	assertContains(t, output, "Disciplined agents build better software")
}

// ── Test helpers ────────────────────────────────────────────────────

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected output to contain %q, got:\n%s", needle, haystack)
	}
}

func assertCallContains(t *testing.T, calls [][]string, keyword string) {
	t.Helper()
	for _, call := range calls {
		for _, arg := range call {
			if strings.Contains(arg, keyword) {
				return
			}
		}
	}
	t.Fatalf("no call contains %q; calls=%v", keyword, calls)
}

func assertSendKeysContains(t *testing.T, calls [][]string, text string) {
	t.Helper()
	for _, call := range calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, text) {
			return
		}
	}
	t.Fatalf("no send-keys call contains %q; calls=%v", text, calls)
}

func countCallsContaining(calls [][]string, keyword string) int {
	count := 0
	for _, call := range calls {
		for _, arg := range call {
			if strings.Contains(arg, keyword) {
				count++
				break
			}
		}
	}
	return count
}

func fileKeys(fs *FakeFS) []string {
	keys := make([]string, 0, len(fs.Files))
	for k := range fs.Files {
		keys = append(keys, k)
	}
	return keys
}
