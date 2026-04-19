package acceptance

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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

// Attach satisfies the forthcoming tmux.Commander.Attach method.
// It records the call so tests can assert ordering.
func (r *RecordingCommander) Attach(session string) error {
	r.Calls = append(r.Calls, []string{"attach-session", "-t", session})
	return nil
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
	assertContains(t, result, "./swarmforge notify")
	assertContains(t, result, "./swarmforge log")
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

// ── C1: E2E-Interpreter prompt scopes to coverage only ─────────────

func TestCLI_E2EInterpreterPromptScopesToCoverage(t *testing.T) {
	// Given a constitution with content "Constitution content"
	// And the agent role is "E2E-Interpreter" with standard instructions
	cfg := prompt.AgentConfig{
		Role:         "E2E-Interpreter",
		Instructions: prompt.E2EInterpreterInstructions,
		Session:      "swarmforge",
		ProjectRoot:  "/project",
	}

	// When the prompt builder generates the prompt
	result := prompt.Build(cfg, "Constitution content")

	// Then the prompt contains the coverage-only phrases
	assertContains(t, result, "cover every Gherkin scenario with a failing end-to-end test")
	assertContains(t, result, "hand off the failing E2E tests to the Coder")

	// And the prompt does not contain the forbidden responsibility claim
	forbidden := "Ensure all Gherkin scenarios pass before any feature is marked complete"
	if strings.Contains(result, forbidden) {
		t.Fatalf("E2E-Interpreter prompt must not contain %q; got:\n%s", forbidden, result)
	}
}

// ── C1: Coder prompt states responsibility for making E2E tests pass ──

func TestCLI_CoderPromptStatesResponsibilityForE2ETests(t *testing.T) {
	// Given a constitution with content "Constitution content"
	// And the agent role is "Coder" with standard instructions
	cfg := prompt.AgentConfig{
		Role:         "Coder",
		Instructions: prompt.CoderInstructions,
		Session:      "swarmforge",
		ProjectRoot:  "/project",
	}

	// When the prompt builder generates the prompt
	result := prompt.Build(cfg, "Constitution content")

	// Then the prompt states responsibility for receiving and passing E2E tests
	assertContains(t, result, "receive failing end-to-end tests from the E2E Interpreter")
	assertContains(t, result, "implement the feature until every E2E test passes")
}

// ── C2: Log entries are separated for readability in the Metrics pane ──

func TestCLI_LogEntriesAreSeparatedForReadability(t *testing.T) {
	// Given a log writer is configured
	var buf bytes.Buffer
	logger := swarmlog.New(&buf)

	// When the user logs a message with role "Architect" and text "task started"
	if err := logger.Write("Architect", "task started"); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Then the log writer output contains a separator line of "========"
	assertContains(t, buf.String(), "========")

	// And the log writer output ends with a newline character
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Fatalf("expected output to end with newline; got: %q", buf.String())
	}

	// When the user logs a second message with role "Coder" and text "tests green"
	if err := logger.Write("Coder", "tests green"); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Then output contains both entries
	output := buf.String()
	assertContains(t, output, "task started")
	assertContains(t, output, "tests green")

	// And the separator "========" appears between the two entries
	firstIdx := strings.Index(output, "task started")
	secondIdx := strings.Index(output, "tests green")
	if firstIdx < 0 || secondIdx < 0 || firstIdx >= secondIdx {
		t.Fatalf("entries missing or out of order in output:\n%s", output)
	}
	between := output[firstIdx:secondIdx]
	if !strings.Contains(between, "========") {
		t.Fatalf("separator '========' must appear between entries; between:\n%s", between)
	}
}

// ── C5 (updated from C4): Notify splits delivery into text + standalone Enter ──

func TestCLI_NotifyAlwaysSubmitsWithEnter(t *testing.T) {
	// Given a recording tmux commander
	var logBuf bytes.Buffer
	logger := swarmlog.New(&logBuf)
	cmd := NewRecordingCommander()

	// When the user runs notify for pane 2 with message "handoff ready"
	if err := notify.Notify(cmd, logger, "swarmforge", 2, "handoff ready",
		tmux.WithSleeper(noopAcceptanceSleeper{})); err != nil {
		t.Fatalf("Notify error: %v", err)
	}

	// Then two "send-keys" invocations are recorded in order
	var sendKeys [][]string
	for _, c := range cmd.Calls {
		if len(c) > 0 && c[0] == "send-keys" {
			sendKeys = append(sendKeys, c)
		}
	}
	if len(sendKeys) != 2 {
		t.Fatalf("notify must produce exactly 2 send-keys invocations; got %d: %v",
			len(sendKeys), sendKeys)
	}

	// And both invocations target "swarmforge:swarm.2"
	for i, c := range sendKeys {
		assertContains(t, strings.Join(c, " "), "swarmforge:swarm.2")
		_ = i
	}

	// And the first invocation uses '-l' and carries the message
	first := sendKeys[0]
	firstHasLiteral := false
	for _, a := range first {
		if a == "-l" {
			firstHasLiteral = true
			break
		}
	}
	if !firstHasLiteral {
		t.Errorf("first invocation must use the '-l' literal flag; got: %v", first)
	}
	assertContains(t, strings.Join(first, " "), "handoff ready")

	// And the first invocation does NOT include the argument "Enter"
	for _, a := range first {
		if a == "Enter" {
			t.Errorf("first invocation must NOT include 'Enter' (that is the second call); got: %v", first)
		}
	}

	// And the second invocation's final argument is exactly "Enter", carrying no payload
	second := sendKeys[1]
	if second[len(second)-1] != "Enter" {
		t.Errorf("second invocation's final arg must be exactly 'Enter'; got %q; full: %v",
			second[len(second)-1], second)
	}
	for _, a := range second {
		if strings.Contains(a, "handoff ready") {
			t.Errorf("second invocation must NOT carry the payload — Enter only; got: %v", second)
		}
	}
}

// ── C5 (updated from C4): SendKeys delivers text and Enter as two tmux calls ──

func TestCLI_SendKeysTerminatesWithEnter(t *testing.T) {
	// Given a recording tmux commander
	cmd := NewRecordingCommander()

	// When SendKeys is called for session "swarmforge" window "swarm" pane 1 with keys "any payload"
	// A sleeper is injected via tmux.WithSleeper so no real time is spent in tests.
	// Note: tests import via the tmux package helpers; see internal/tmux/tmux_test.go
	// for the recording sleeper definition. This acceptance test uses a minimal no-op.
	noop := noopAcceptanceSleeper{}
	if err := tmux.SendKeys(cmd, "swarmforge", "swarm", 1, "any payload",
		tmux.WithSleeper(noop)); err != nil {
		t.Fatalf("SendKeys error: %v", err)
	}

	// Then exactly two tmux invocations are recorded
	if len(cmd.Calls) != 2 {
		t.Fatalf("SendKeys must make exactly 2 tmux calls; got %d: %v", len(cmd.Calls), cmd.Calls)
	}

	// And the first is '-l' + payload, second is standalone 'Enter'
	first := cmd.Calls[0]
	firstHasLiteral := false
	for _, a := range first {
		if a == "-l" {
			firstHasLiteral = true
			break
		}
	}
	if !firstHasLiteral {
		t.Errorf("first call must include '-l'; got: %v", first)
	}
	assertContains(t, strings.Join(first, " "), "any payload")

	second := cmd.Calls[1]
	if len(second) == 0 || second[len(second)-1] != "Enter" {
		t.Errorf("second call's final arg must be exactly 'Enter'; got: %v", second)
	}
	// Omitting either the '-l' text call or the trailing standalone Enter call
	// is a violation of the handoff contract (this test's failures ARE the violation).
}

// ── C3: Start sequence attaches the user to the tmux session after launch ──

func TestCLI_StartAttachesToSessionAsFinalStep(t *testing.T) {
	// Given no tmux session named "swarmforge" exists
	cmd := NewRecordingCommander()
	fs := NewFakeFS()
	fs.Files["/project/Constitution.md"] = []byte("constitution")
	var stdout bytes.Buffer

	// When the start sequence completes all setup steps
	cfgStart := start.Config{
		Commander:        cmd,
		Session:          "swarmforge",
		ProjectRoot:      "/project",
		FS:               fs,
		LookPath:         func(name string) (string, error) { return "/usr/bin/" + name, nil },
		ConstitutionPath: "Constitution.md",
		Stdout:           &stdout,
	}
	if err := start.Run(cfgStart); err != nil {
		t.Fatalf("start.Run error: %v", err)
	}

	// Then the commander attaches to session "swarmforge"
	attachIdx := lastIndexOfCallContaining(cmd.Calls, "attach-session")
	if attachIdx < 0 {
		t.Fatalf("commander must attach to 'swarmforge' session; calls=%v", cmd.Calls)
	}
	attachJoined := strings.Join(cmd.Calls[attachIdx], " ")
	if !strings.Contains(attachJoined, "swarmforge") {
		t.Fatalf("attach call must target 'swarmforge'; got: %v", cmd.Calls[attachIdx])
	}

	// And the attach step occurs after the metrics pane is initialized
	metricsIdx := lastIndexOfCallContaining(cmd.Calls, "tail -f logs/agent_messages.log")
	if metricsIdx < 0 {
		t.Fatalf("metrics pane init missing; calls=%v", cmd.Calls)
	}
	if attachIdx <= metricsIdx {
		t.Fatalf("attach must come after metrics pane init; attachIdx=%d metricsIdx=%d",
			attachIdx, metricsIdx)
	}

	// And the attach step is the final action of the start sequence
	if attachIdx != len(cmd.Calls)-1 {
		t.Fatalf("attach must be the final call; attachIdx=%d total=%d calls=%v",
			attachIdx, len(cmd.Calls), cmd.Calls)
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

// lastIndexOfCallContaining returns the index of the last call whose args contain keyword, or -1.
func lastIndexOfCallContaining(calls [][]string, keyword string) int {
	idx := -1
	for i, call := range calls {
		for _, arg := range call {
			if strings.Contains(arg, keyword) {
				idx = i
				break
			}
		}
	}
	return idx
}

// noopAcceptanceSleeper satisfies tmux.Sleeper without doing any wall-clock sleeping.
// Acceptance-layer tests use this to prove the two-call pattern without burning 200ms
// per SendKeys invocation.
type noopAcceptanceSleeper struct{}

func (noopAcceptanceSleeper) Sleep(_ time.Duration) {}

