package tmux_test

import (
	"strings"
	"testing"
	"time"

	"github.com/swarm-forge/swarm-forge/internal/tmux"
)

// recSleeper records every pause it is asked to wait. Tests use it to assert
// ordering and duration of the inter-call pause without any wall-clock sleep.
type recSleeper struct {
	durations []time.Duration
	// callIndexAtSleep records len(recCmd.calls) at the moment Sleep runs so
	// tests can assert the sleep landed between the first and second tmux invocations.
	callIndexAtSleep []int
	cmd              *recCmd // optional — set via withCmd for ordering assertions.
}

func (r *recSleeper) Sleep(d time.Duration) {
	r.durations = append(r.durations, d)
	if r.cmd != nil {
		r.callIndexAtSleep = append(r.callIndexAtSleep, len(r.cmd.calls))
	}
}

func (r *recSleeper) withCmd(c *recCmd) *recSleeper { r.cmd = c; return r }

// noopSleeper ignores Sleep — used to prove SendKeys does not introduce
// wall-clock delay when the sleeper is a no-op.
type noopSleeper struct{ calls int }

func (n *noopSleeper) Sleep(_ time.Duration) { n.calls++ }

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

// Attach satisfies the Commander interface once Coder adds Attach(session string) error.
// Recording it in calls lets tests assert ordering relative to other tmux commands.
func (r *recCmd) Attach(session string) error {
	r.calls = append(r.calls, []string{"attach-session", "-t", session})
	return nil
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

func countCalls(calls [][]string, keyword string) int {
	n := 0
	for _, c := range calls {
		for _, a := range c {
			if strings.Contains(a, keyword) {
				n++
				break
			}
		}
	}
	return n
}

func TestCreateSession(t *testing.T) {
	cmd := newRecCmd()
	err := tmux.CreateSession(cmd, "sf", "swarm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasCall(cmd.calls, "new-session") {
		t.Fatal("missing new-session call")
	}
}

func TestSplitGrid(t *testing.T) {
	cmd := newRecCmd()
	err := tmux.SplitGrid(cmd, "sf", "swarm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if countCalls(cmd.calls, "split-window") != 3 {
		t.Fatalf("expected 3 split-window calls, got %d", countCalls(cmd.calls, "split-window"))
	}
}

func TestSetPaneTitles(t *testing.T) {
	cmd := newRecCmd()
	err := tmux.SetPaneTitles(cmd, "sf", "swarm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasCall(cmd.calls, "select-pane") {
		t.Fatal("missing select-pane call")
	}
}

func TestKillSession(t *testing.T) {
	cmd := newRecCmd()
	err := tmux.KillSession(cmd, "sf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasCall(cmd.calls, "kill-session") {
		t.Fatal("missing kill-session call")
	}
}

func TestLaunchAgent(t *testing.T) {
	cmd := newRecCmd()
	err := tmux.LaunchAgent(cmd, "sf", 0, "Architect", "/tmp/prompt.md", "/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasCall(cmd.calls, "send-keys") {
		t.Fatal("missing send-keys call")
	}
	if !hasCall(cmd.calls, "SwarmForge Architect") {
		t.Fatal("missing agent name in command")
	}
	if !hasCall(cmd.calls, "--permission-mode acceptEdits") {
		t.Fatal("missing permission mode")
	}
}

func TestSendKeys(t *testing.T) {
	cmd := newRecCmd()
	err := tmux.SendKeys(cmd, "sf", "swarm", 3, "tail -f log")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasCall(cmd.calls, "send-keys") {
		t.Fatal("missing send-keys call")
	}
	if !hasCall(cmd.calls, "tail -f log") {
		t.Fatal("missing command text")
	}
}

// C4 invariant (updated for C5): SendKeys now emits TWO tmux invocations —
// first the text (with -l literal flag), then a standalone Enter. The
// Enter-as-final-delivery invariant is preserved: it is the final arg of the
// final tmux call to the pane. Do not weaken this assertion.
func TestSendKeysFinalArgIsLiteralEnter(t *testing.T) {
	cmd := newRecCmd()
	sleeper := &recSleeper{}
	if err := tmux.SendKeys(cmd, "swarmforge", "swarm", 1, "any payload",
		tmux.WithSleeper(sleeper)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.calls) != 2 {
		t.Fatalf("expected exactly two tmux invocations (text then standalone Enter); got %d: %v",
			len(cmd.calls), cmd.calls)
	}
	last := cmd.calls[len(cmd.calls)-1]
	if len(last) == 0 {
		t.Fatal("last recorded call has no args")
	}
	if last[len(last)-1] != "Enter" {
		t.Fatalf("final tmux call's last arg must be 'Enter'; got %q; full call: %v",
			last[len(last)-1], last)
	}
}

// C5 — SendKeys splits delivery into a text call then a standalone Enter call.
// Claude Code's input box needs this gap to register the pasted text before
// the submit key arrives.
func TestSendKeysSplitsTextAndEnter(t *testing.T) {
	cmd := newRecCmd()
	sleeper := &recSleeper{}
	if err := tmux.SendKeys(cmd, "swarmforge", "swarm", 1, "any payload",
		tmux.WithSleeper(sleeper)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.calls) != 2 {
		t.Fatalf("expected exactly 2 tmux invocations; got %d: %v", len(cmd.calls), cmd.calls)
	}

	first := cmd.calls[0]
	if !containsArg(first, "-l") {
		t.Errorf("first tmux call must use the literal-text flag '-l'; got: %v", first)
	}
	if !containsArg(first, "any payload") {
		t.Errorf("first tmux call must carry the payload; got: %v", first)
	}
	if containsArg(first, "Enter") {
		t.Errorf("first tmux call must NOT include 'Enter' (that is the second call); got: %v", first)
	}

	second := cmd.calls[1]
	if second[len(second)-1] != "Enter" {
		t.Errorf("second tmux call's final arg must be exactly 'Enter'; got: %v", second)
	}
	if containsArg(second, "any payload") {
		t.Errorf("second tmux call must NOT carry the payload — Enter only; got: %v", second)
	}
}

// C5 — a single Sleep happens between the two tmux invocations.
func TestSendKeysPausesBetweenCalls(t *testing.T) {
	cmd := newRecCmd()
	sleeper := (&recSleeper{}).withCmd(cmd)
	if err := tmux.SendKeys(cmd, "swarmforge", "swarm", 1, "msg",
		tmux.WithSleeper(sleeper),
		tmux.WithPause(250*time.Millisecond)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sleeper.durations) != 1 {
		t.Fatalf("sleeper must be called exactly once; got %d calls with durations %v",
			len(sleeper.durations), sleeper.durations)
	}
	if sleeper.durations[0] != 250*time.Millisecond {
		t.Errorf("expected 250ms pause; got %v", sleeper.durations[0])
	}
	if len(sleeper.callIndexAtSleep) != 1 || sleeper.callIndexAtSleep[0] != 1 {
		t.Errorf("sleeper must run AFTER the first tmux call and BEFORE the second; "+
			"len(calls) at Sleep=%v, want 1", sleeper.callIndexAtSleep)
	}
}

// C5 — unconfigured pause defaults to exactly 200ms.
func TestSendKeysDefaultPauseIs200ms(t *testing.T) {
	cmd := newRecCmd()
	sleeper := &recSleeper{}
	if err := tmux.SendKeys(cmd, "swarmforge", "swarm", 1, "msg",
		tmux.WithSleeper(sleeper)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sleeper.durations) != 1 {
		t.Fatalf("sleeper must be called once; got %d", len(sleeper.durations))
	}
	if sleeper.durations[0] != 200*time.Millisecond {
		t.Errorf("default pause must be exactly 200ms; got %v", sleeper.durations[0])
	}
}

// C5 — pause is configurable per call; values are threaded through without rounding.
func TestSendKeysConfigurablePause(t *testing.T) {
	for _, p := range []time.Duration{0, 50 * time.Millisecond, 750 * time.Millisecond} {
		cmd := newRecCmd()
		sleeper := &recSleeper{}
		if err := tmux.SendKeys(cmd, "swarmforge", "swarm", 1, "msg",
			tmux.WithSleeper(sleeper),
			tmux.WithPause(p)); err != nil {
			t.Fatalf("pause=%v: unexpected error: %v", p, err)
		}
		if len(sleeper.durations) != 1 {
			t.Fatalf("pause=%v: expected exactly one Sleep; got %d", p, len(sleeper.durations))
		}
		if sleeper.durations[0] != p {
			t.Errorf("pause=%v: got %v", p, sleeper.durations[0])
		}
	}
}

// C5 — literal flag protects payloads from tmux key-name interpretation.
// A payload containing the token 'Enter' must be typed as five characters
// during the FIRST send-keys; only the SECOND send-keys carries the actual
// Enter key.
func TestSendKeysLiteralFlagAvoidsKeyNameInterpretation(t *testing.T) {
	cmd := newRecCmd()
	sleeper := &recSleeper{}
	if err := tmux.SendKeys(cmd, "swarmforge", "swarm", 1, "say Enter again",
		tmux.WithSleeper(sleeper)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.calls) != 2 {
		t.Fatalf("expected 2 calls; got %d: %v", len(cmd.calls), cmd.calls)
	}

	first := cmd.calls[0]
	if !containsArg(first, "-l") {
		t.Errorf("first call must include '-l'; got: %v", first)
	}
	if !containsArg(first, "say Enter again") {
		t.Errorf("first call must carry the literal payload; got: %v", first)
	}
	for i, arg := range first {
		if arg == "Enter" {
			t.Errorf("first call must NOT contain 'Enter' as a standalone key arg (pos %d); got: %v", i, first)
		}
	}

	second := cmd.calls[1]
	if len(second) == 0 || second[len(second)-1] != "Enter" {
		t.Errorf("second call's final arg must be exactly 'Enter'; got: %v", second)
	}
	// Second call carries ONLY Enter (target args aside).
	for _, arg := range second {
		if strings.Contains(arg, "say Enter again") {
			t.Errorf("second call must NOT carry the payload; got: %v", second)
		}
	}
}

// C5 — substituting a no-op sleeper eliminates wall-clock delay across many
// calls. Proves the Sleeper is actually the injection point (not a bypass).
func TestSendKeysSleeperIsInjectable(t *testing.T) {
	cmd := newRecCmd()
	noop := &noopSleeper{}

	start := time.Now()
	const iterations = 200
	for i := 0; i < iterations; i++ {
		if err := tmux.SendKeys(cmd, "swarmforge", "swarm", 1, "x",
			tmux.WithSleeper(noop),
			tmux.WithPause(500*time.Millisecond)); err != nil {
			t.Fatalf("iter %d: unexpected error: %v", i, err)
		}
	}
	elapsed := time.Since(start)

	if noop.calls != iterations {
		t.Errorf("no-op sleeper should be called once per SendKeys; got %d for %d iterations",
			noop.calls, iterations)
	}
	// Conservative bound: 200 iterations * 500ms pause would be >= 100 seconds
	// if the sleeper were bypassed. Requiring under one second proves injection.
	if elapsed > time.Second {
		t.Errorf("with no-op sleeper, %d SendKeys calls must complete quickly; took %v",
			iterations, elapsed)
	}
	if len(cmd.calls) != iterations*2 {
		t.Errorf("expected %d tmux calls (2 per SendKeys); got %d", iterations*2, len(cmd.calls))
	}
}

// containsArg returns true if any element of call equals arg exactly.
func containsArg(call []string, arg string) bool {
	for _, a := range call {
		if a == arg {
			return true
		}
	}
	return false
}
