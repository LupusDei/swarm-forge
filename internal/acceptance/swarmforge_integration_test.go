package acceptance

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/swarm-forge/swarm-forge/internal/notify"
	"github.com/swarm-forge/swarm-forge/internal/start"
	"github.com/swarm-forge/swarm-forge/internal/swarmlog"
	"github.com/swarm-forge/swarm-forge/internal/tmux"
)

// ── Scenario 1: Start sequence performs complete startup ────────────

func TestIntegration_StartRunPerformsCompleteStartup(t *testing.T) {
	cmd := NewRecordingCommander()
	fs := NewFakeFS()
	var stdout bytes.Buffer

	// Given a fake filesystem contains a constitution file
	constitutionContent := "Rule 1: TDD is mandatory\nRule 2: Gherkin is truth"
	fs.Files["/project/Constitution.md"] = []byte(constitutionContent)

	// Given all dependencies are available
	lookPath := func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}

	// When start.Run is called with the full configuration
	cfg := start.Config{
		Commander:        cmd,
		Session:          "swarmforge",
		ProjectRoot:      "/project",
		FS:               fs,
		LookPath:         lookPath,
		ConstitutionPath: "Constitution.md",
		Stdout:           &stdout,
	}
	err := start.Run(cfg)
	if err != nil {
		t.Fatalf("start.Run error: %v", err)
	}

	// Then preflight checks are performed for "tmux", "claude", and "watch"
	// (no error means they passed — tested via lookPath being called)

	// And directory setup creates "features", "logs", and "agent_context"
	for _, dir := range []string{"features", "logs", "agent_context"} {
		expected := "/project/" + dir
		found := false
		for _, d := range fs.Dirs {
			if d == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("directory %q not created; dirs=%v", expected, fs.Dirs)
		}
	}

	// And the startup banner is printed to stdout
	bannerOutput := stdout.String()
	if !strings.Contains(bannerOutput, "SwarmForge") {
		t.Errorf("banner missing 'SwarmForge'; stdout=%q", bannerOutput)
	}
	if !strings.Contains(bannerOutput, "Disciplined agents") {
		t.Errorf("banner missing motto; stdout=%q", bannerOutput)
	}

	// And a tmux session "swarmforge" with window "swarm" is created
	assertCallContainsInteg(t, cmd.Calls, "new-session")

	// And the session is split into a 2x2 grid
	splitCount := countCallsContainingInteg(cmd.Calls, "split-window")
	if splitCount != 3 {
		t.Errorf("expected 3 split-window calls, got %d", splitCount)
	}

	// And pane titles are set for all 4 panes
	selectPaneCount := countCallsContainingInteg(cmd.Calls, "select-pane")
	if selectPaneCount < 4 {
		t.Errorf("expected >= 4 select-pane calls, got %d", selectPaneCount)
	}

	// And agent prompt files are written for "Architect", "Coder", "E2E-Interpreter"
	promptNames := []string{"Architect", "Coder", "E2E-Interpreter"}
	for _, name := range promptNames {
		promptPath := "/project/.swarmforge/prompts/" + name + ".md"
		data, ok := fs.Files[promptPath]
		if !ok {
			t.Errorf("prompt file not written for %s at %s", name, promptPath)
			continue
		}
		content := string(data)

		// And each prompt file contains the constitution content
		if !strings.Contains(content, "Rule 1: TDD is mandatory") {
			t.Errorf("prompt for %s missing constitution content", name)
		}

		// And each prompt file contains coordination instructions
		if !strings.Contains(content, "./swarmforge notify") {
			t.Errorf("prompt for %s missing coordination instructions", name)
		}
	}

	// And claude is launched in panes 0, 1, 2 with correct names
	assertSendKeysContainsInteg(t, cmd.Calls, "SwarmForge Architect")
	assertSendKeysContainsInteg(t, cmd.Calls, "SwarmForge E2E-Interpreter")
	assertSendKeysContainsInteg(t, cmd.Calls, "SwarmForge Coder")

	// And each launch includes --permission-mode acceptEdits
	for _, call := range cmd.Calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "claude") && strings.Contains(joined, "send-keys") {
			if !strings.Contains(joined, "--permission-mode acceptEdits") {
				t.Errorf("claude launch missing --permission-mode acceptEdits: %v", call)
			}
		}
	}

	// And pane 3 receives "tail -f logs/agent_messages.log"
	foundTail := false
	tailIdx := -1
	for i, call := range cmd.Calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "tail -f logs/agent_messages.log") {
			foundTail = true
			tailIdx = i
			break
		}
	}
	if !foundTail {
		t.Error("pane 3 did not receive tail command")
	}

	// And after all panes are initialized the commander attaches to session "swarmforge"
	attachIdx := -1
	for i, call := range cmd.Calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "attach-session") && strings.Contains(joined, "swarmforge") {
			attachIdx = i
		}
	}
	if attachIdx < 0 {
		t.Error("start.Run must attach to 'swarmforge' session as final step")
	}
	if tailIdx >= 0 && attachIdx <= tailIdx {
		t.Errorf("attach-session must come after metrics pane init; attachIdx=%d tailIdx=%d", attachIdx, tailIdx)
	}
	if attachIdx >= 0 && attachIdx != len(cmd.Calls)-1 {
		t.Errorf("attach-session must be the final call; attachIdx=%d total=%d", attachIdx, len(cmd.Calls))
	}
}

// ── Scenario 2: Start kills existing session before creating ────────

func TestIntegration_StartKillsExistingBeforeNew(t *testing.T) {
	// Given a recording commander that reports session "swarmforge" exists
	cmd := NewRecordingCommander()
	cmd.Sessions["swarmforge"] = true

	fs := NewFakeFS()
	fs.Files["/project/Constitution.md"] = []byte("constitution")

	lookPath := func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	var stdout bytes.Buffer

	// When start.Run is called with the full configuration
	cfg := start.Config{
		Commander:        cmd,
		Session:          "swarmforge",
		ProjectRoot:      "/project",
		FS:               fs,
		LookPath:         lookPath,
		ConstitutionPath: "Constitution.md",
		Stdout:           &stdout,
	}
	err := start.Run(cfg)
	if err != nil {
		t.Fatalf("start.Run error: %v", err)
	}

	// Then "kill-session" is called before "new-session"
	killIdx := indexOfCallContaining(cmd.Calls, "kill-session")
	newIdx := indexOfCallContaining(cmd.Calls, "new-session")
	if killIdx < 0 {
		t.Fatal("kill-session was not called")
	}
	if newIdx < 0 {
		t.Fatal("new-session was not called")
	}
	if killIdx >= newIdx {
		t.Fatalf("kill-session (index %d) must come before new-session (index %d)", killIdx, newIdx)
	}
}

// ── Scenario 3: Start fails fast on missing dependency ──────────────

func TestIntegration_StartFailsFastOnMissingDep(t *testing.T) {
	cmd := NewRecordingCommander()
	fs := NewFakeFS()
	fs.Files["/project/Constitution.md"] = []byte("constitution")
	var stdout bytes.Buffer

	// Given a lookpath function that rejects "claude"
	lookPath := func(name string) (string, error) {
		if name == "claude" {
			return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
		}
		return "/usr/bin/" + name, nil
	}

	// When start.Run is called with the full configuration
	cfg := start.Config{
		Commander:        cmd,
		Session:          "swarmforge",
		ProjectRoot:      "/project",
		FS:               fs,
		LookPath:         lookPath,
		ConstitutionPath: "Constitution.md",
		Stdout:           &stdout,
	}
	err := start.Run(cfg)

	// Then an error is returned containing "claude"
	if err == nil {
		t.Fatal("expected error for missing claude, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "claude") {
		t.Fatalf("error should mention claude, got: %s", err.Error())
	}

	// And no tmux commands are executed
	if len(cmd.Calls) > 0 {
		t.Fatalf("expected no tmux calls after preflight failure, got: %v", cmd.Calls)
	}
}

// ── Scenario 4: Start fails if constitution file is missing ─────────

func TestIntegration_StartFailsOnMissingConstitution(t *testing.T) {
	cmd := NewRecordingCommander()
	fs := NewFakeFS() // no constitution file
	var stdout bytes.Buffer

	lookPath := func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}

	// When start.Run is called with the full configuration
	cfg := start.Config{
		Commander:        cmd,
		Session:          "swarmforge",
		ProjectRoot:      "/project",
		FS:               fs,
		LookPath:         lookPath,
		ConstitutionPath: "Constitution.md",
		Stdout:           &stdout,
	}
	err := start.Run(cfg)

	// Then an error is returned containing "constitution"
	if err == nil {
		t.Fatal("expected error for missing constitution, got nil")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "constitution") {
		t.Fatalf("error should mention constitution, got: %s", err.Error())
	}

	// And no tmux commands are executed
	if len(cmd.Calls) > 0 {
		t.Fatalf("expected no tmux calls after constitution error, got: %v", cmd.Calls)
	}
}

// ── Scenario 5: Real commander wraps os/exec ────────────────────────

func TestIntegration_ExecCommanderExists(t *testing.T) {
	// Given a real commander implementation from the tmux package
	// This test verifies the type exists and satisfies the Commander interface
	var cmd tmux.Commander = tmux.NewExecCommander()
	if cmd == nil {
		t.Fatal("NewExecCommander returned nil")
	}

	// When Run is called with arguments "list-sessions"
	// (We don't assert success since tmux may not be running,
	//  but the method must exist and be callable)
	_ = cmd.Run("list-sessions")

	// HasSession must also be callable
	_ = cmd.HasSession("nonexistent-session-name")
}

// ── Scenario 6: CLI binary builds ───────────────────────────────────

func TestIntegration_CLIBinaryBuilds(t *testing.T) {
	// Given the cmd/swarmforge package exists with a main.go
	// When "go build github.com/swarm-forge/swarm-forge/cmd/swarmforge" is run
	out, err := exec.Command("go", "build", "-o", "/dev/null",
		"github.com/swarm-forge/swarm-forge/cmd/swarmforge").
		CombinedOutput()

	// Then the build succeeds with exit code 0
	if err != nil {
		t.Fatalf("go build failed: %v\noutput: %s", err, string(out))
	}
}

// ── Scenario 7: Notify wires commander and logger end-to-end ────────

func TestIntegration_NotifyWiresCommanderAndLogger(t *testing.T) {
	// Given a recording commander and a log writer
	cmd := NewRecordingCommander()
	var logBuf bytes.Buffer
	logger := swarmlog.New(&logBuf)

	// When the notify handler is called with pane "0" and message "hello"
	err := notify.Notify(cmd, logger, "swarmforge", 0, "hello")
	if err != nil {
		t.Fatalf("Notify error: %v", err)
	}

	// Then the log writer contains "pane 0" and "hello"
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "pane 0") {
		t.Errorf("log missing 'pane 0': %q", logOutput)
	}
	if !strings.Contains(logOutput, "hello") {
		t.Errorf("log missing 'hello': %q", logOutput)
	}

	// And tmux send-keys is called for pane 0 in session "swarmforge"
	assertCallContainsInteg(t, cmd.Calls, "send-keys")
	foundPane := false
	for _, call := range cmd.Calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "send-keys") && strings.Contains(joined, "swarmforge") {
			foundPane = true
			break
		}
	}
	if !foundPane {
		t.Error("send-keys not targeting swarmforge session")
	}
}

// ── Scenario 8: Log writes to both file and stdout ──────────────────

func TestIntegration_LogWritesToFileAndStdout(t *testing.T) {
	// Given a file writer and a stdout writer
	var fileBuf bytes.Buffer
	var stdBuf bytes.Buffer
	logger := swarmlog.New(&fileBuf, &stdBuf)

	// When the log handler is called with role "Architect" and message "done"
	err := logger.Write("Architect", "done")
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Then the file writer contains "[Architect] done"
	if !strings.Contains(fileBuf.String(), "[Architect] done") {
		t.Errorf("file writer missing expected content: %q", fileBuf.String())
	}

	// And the stdout writer contains "[Architect] done"
	if !strings.Contains(stdBuf.String(), "[Architect] done") {
		t.Errorf("stdout writer missing expected content: %q", stdBuf.String())
	}
}

// ── Integration-specific helpers ────────────────────────────────────

func assertCallContainsInteg(t *testing.T, calls [][]string, keyword string) {
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

func assertSendKeysContainsInteg(t *testing.T, calls [][]string, text string) {
	t.Helper()
	for _, call := range calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, text) {
			return
		}
	}
	t.Fatalf("no send-keys call contains %q; calls=%v", text, calls)
}

func countCallsContainingInteg(calls [][]string, keyword string) int {
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

func indexOfCallContaining(calls [][]string, keyword string) int {
	for i, call := range calls {
		for _, arg := range call {
			if strings.Contains(arg, keyword) {
				return i
			}
		}
	}
	return -1
}
