package swarmlog

import (
	"fmt"
	"io"
	"time"
)

// Clock returns the current time. Injectable so tests can freeze it.
type Clock func() time.Time

// Logger writes timestamped log entries to one or more writers.
type Logger struct {
	writers []io.Writer
	clock   Clock
}

// New creates a Logger that writes to all supplied writers. Timestamps
// come from time.Now by default; use SetClock to override in tests.
func New(writers ...io.Writer) *Logger {
	return &Logger{writers: writers, clock: time.Now}
}

// SetClock overrides the timestamp source. Intended for tests.
func (l *Logger) SetClock(c Clock) {
	l.clock = c
}

// Write formats and writes a log entry with the given role and message.
// Format: "[YYYY-MM-DD HH:MM:SS] [role] message\n========\n" — matches
// the format previously produced by the bash helper scripts.
func (l *Logger) Write(role, message string) error {
	ts := l.clock().Format(time.DateTime)
	entry := fmt.Sprintf("[%s] [%s] %s\n========\n", ts, role, message)
	for _, w := range l.writers {
		if _, err := fmt.Fprint(w, entry); err != nil {
			return err
		}
	}
	return nil
}
