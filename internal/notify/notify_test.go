package notify_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/swarm-forge/swarm-forge/internal/notify"
	"github.com/swarm-forge/swarm-forge/internal/swarmlog"
	"github.com/swarm-forge/swarm-forge/internal/tmux"
)

type noopSleeper struct{}

func (noopSleeper) Sleep(_ time.Duration) {}

type recCmd struct {
	calls    [][]string
	sessions map[string]bool
}

func newRecCmd() *recCmd {
	return &recCmd{sessions: map[string]bool{"sf": true}}
}

func (r *recCmd) Run(args ...string) error {
	r.calls = append(r.calls, args)
	return nil
}

func (r *recCmd) HasSession(name string) bool {
	return r.sessions[name]
}

func (r *recCmd) Attach(session string) error {
	r.calls = append(r.calls, []string{"attach-session", "-t", session})
	return nil
}

func TestNotifyLogsAndSends(t *testing.T) {
	var buf bytes.Buffer
	logger := swarmlog.New(&buf)
	cmd := newRecCmd()
	err := notify.Notify(cmd, logger, "sf", 0, "hello architect")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "[pane 0] hello architect") {
		t.Fatalf("missing log entry: %s", buf.String())
	}
	found := false
	for _, c := range cmd.calls {
		for _, a := range c {
			if strings.Contains(a, "send-keys") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("missing send-keys call")
	}
}

// C4 invariant (updated for C5): Notify must ultimately produce TWO send-keys
// tmux invocations to the target pane — first the literal-flag text, then a
// standalone Enter. The Enter-as-final-delivery invariant is preserved: it is
// the final arg of the final send-keys call. Any refactor that drops Enter
// or collapses back to a single call MUST make this test go red.
func TestNotifyFinalArgIsLiteralEnter(t *testing.T) {
	var buf bytes.Buffer
	logger := swarmlog.New(&buf)
	cmd := newRecCmd()
	if err := notify.Notify(cmd, logger, "swarmforge", 2, "handoff ready",
		tmux.WithSleeper(noopSleeper{})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sendKeys [][]string
	for _, c := range cmd.calls {
		if len(c) > 0 && c[0] == "send-keys" {
			sendKeys = append(sendKeys, c)
		}
	}
	if len(sendKeys) != 2 {
		t.Fatalf("notify must produce exactly 2 send-keys invocations (text then Enter); got %d: %v",
			len(sendKeys), sendKeys)
	}

	first := sendKeys[0]
	firstJoined := strings.Join(first, " ")
	if !strings.Contains(firstJoined, "swarmforge:swarm.2") {
		t.Errorf("first send-keys must target swarmforge:swarm.2; got: %v", first)
	}
	if !strings.Contains(firstJoined, "handoff ready") {
		t.Errorf("first send-keys must carry the message; got: %v", first)
	}
	firstHasLiteralFlag := false
	for _, a := range first {
		if a == "-l" {
			firstHasLiteralFlag = true
			break
		}
	}
	if !firstHasLiteralFlag {
		t.Errorf("first send-keys must include the '-l' literal flag; got: %v", first)
	}

	last := sendKeys[len(sendKeys)-1]
	if last[len(last)-1] != "Enter" {
		t.Errorf("final send-keys' last arg must be exactly 'Enter'; got %q; full: %v",
			last[len(last)-1], last)
	}
	for _, a := range last {
		if strings.Contains(a, "handoff ready") {
			t.Errorf("final send-keys must NOT carry the payload — Enter only; got: %v", last)
		}
	}
}
