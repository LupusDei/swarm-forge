package swarmlog_test

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/swarm-forge/swarm-forge/internal/swarmlog"
)

func TestWriteFormatsRoleAndMessage(t *testing.T) {
	var buf bytes.Buffer
	logger := swarmlog.New(&buf)
	err := logger.Write("Architect", "task started")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "[Architect] task started") {
		t.Fatalf("missing formatted entry: %s", buf.String())
	}
}

func TestWriteMultipleWriters(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	logger := swarmlog.New(&buf1, &buf2)
	err := logger.Write("Coder", "done")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf1.String(), "[Coder] done") {
		t.Fatalf("writer 1 missing entry: %s", buf1.String())
	}
	if !strings.Contains(buf2.String(), "[Coder] done") {
		t.Fatalf("writer 2 missing entry: %s", buf2.String())
	}
}

// C2 red test — output must contain the '========' separator line.
func TestWriteIncludesSeparatorLine(t *testing.T) {
	var buf bytes.Buffer
	logger := swarmlog.New(&buf)
	if err := logger.Write("Architect", "task started"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "========") {
		t.Fatalf("expected separator line '========' in output, got:\n%s", buf.String())
	}
}

// C2 red test — output must end with a newline character.
func TestWriteOutputEndsWithNewline(t *testing.T) {
	var buf bytes.Buffer
	logger := swarmlog.New(&buf)
	if err := logger.Write("Architect", "task started"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.HasSuffix(output, "\n") {
		t.Fatalf("expected output to end with newline, got: %q", output)
	}
}

// Timestamp prefix invariant — entries MUST begin with a [YYYY-MM-DD HH:MM:SS]
// stamp so `logs/agent_messages.log` stays debuggable. Drop the timestamp and
// this test goes red.
func TestWriteIncludesTimestampPrefix(t *testing.T) {
	var buf bytes.Buffer
	logger := swarmlog.New(&buf)
	fixed := time.Date(2026, 4, 19, 14, 23, 45, 0, time.UTC)
	logger.SetClock(func() time.Time { return fixed })
	if err := logger.Write("Architect", "task started"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "[2026-04-19 14:23:45] [Architect] task started") {
		t.Fatalf("expected '[2026-04-19 14:23:45] [Architect] task started' prefix; got:\n%s", got)
	}
}

// Default clock sanity check — no SetClock means time.Now, which still matches
// the "[YYYY-MM-DD HH:MM:SS]" shape.
func TestWriteDefaultClockProducesTimestamp(t *testing.T) {
	var buf bytes.Buffer
	logger := swarmlog.New(&buf)
	if err := logger.Write("Coder", "ok"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	re := regexp.MustCompile(`^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\] \[Coder\] ok`)
	if !re.MatchString(buf.String()) {
		t.Fatalf("entry missing timestamp prefix; got:\n%s", buf.String())
	}
}

// C2 red test — separator must appear between two consecutive entries.
func TestWriteSeparatorAppearsBetweenEntries(t *testing.T) {
	var buf bytes.Buffer
	logger := swarmlog.New(&buf)
	if err := logger.Write("Architect", "task started"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := logger.Write("Coder", "tests green"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	firstIdx := strings.Index(output, "task started")
	secondIdx := strings.Index(output, "tests green")
	if firstIdx < 0 {
		t.Fatalf("missing first entry in output:\n%s", output)
	}
	if secondIdx < 0 {
		t.Fatalf("missing second entry in output:\n%s", output)
	}
	if firstIdx >= secondIdx {
		t.Fatalf("entries appear out of order:\n%s", output)
	}
	between := output[firstIdx:secondIdx]
	if !strings.Contains(between, "========") {
		t.Fatalf("separator '========' must appear between the two entries; between:\n%s", between)
	}
}
