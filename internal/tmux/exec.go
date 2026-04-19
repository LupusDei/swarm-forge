package tmux

import (
	"os"
	"os/exec"
)

// ExecCommander runs tmux commands via os/exec.
type ExecCommander struct{}

// NewExecCommander creates a Commander that shells out to tmux.
func NewExecCommander() *ExecCommander {
	return &ExecCommander{}
}

// Run executes tmux with the given arguments.
func (e *ExecCommander) Run(args ...string) error {
	return exec.Command("tmux", args...).Run()
}

// HasSession returns true if tmux reports the named session exists.
func (e *ExecCommander) HasSession(name string) bool {
	return exec.Command("tmux", "has-session", "-t", name).Run() == nil
}

// Attach blocks while the user is attached to the named tmux session.
func (e *ExecCommander) Attach(session string) error {
	cmd := exec.Command("tmux", "attach-session", "-t", session)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
