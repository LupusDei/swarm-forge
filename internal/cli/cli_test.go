package cli_test

import (
	"testing"

	"github.com/swarm-forge/swarm-forge/internal/cli"
)

func TestDispatchStart(t *testing.T) {
	called := false
	cfg := cli.Config{
		Start:  func(_ []string) error { called = true; return nil },
		Notify: func(_ []string) error { return nil },
		Log:    func(_ []string) error { return nil },
	}
	err := cli.Dispatch([]string{"start"}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("start not called")
	}
}

func TestDispatchNotify(t *testing.T) {
	called := false
	cfg := cli.Config{
		Start:  func(_ []string) error { return nil },
		Notify: func(_ []string) error { called = true; return nil },
		Log:    func(_ []string) error { return nil },
	}
	err := cli.Dispatch([]string{"notify", "1", "hi"}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("notify not called")
	}
}

func TestDispatchLog(t *testing.T) {
	called := false
	cfg := cli.Config{
		Start:  func(_ []string) error { return nil },
		Notify: func(_ []string) error { return nil },
		Log:    func(_ []string) error { called = true; return nil },
	}
	err := cli.Dispatch([]string{"log", "Coder", "done"}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("log not called")
	}
}

func TestDispatchCrap(t *testing.T) {
	var gotArgs []string
	called := false
	cfg := cli.Config{
		Start:  func(_ []string) error { return nil },
		Notify: func(_ []string) error { return nil },
		Log:    func(_ []string) error { return nil },
		Crap: func(args []string) error {
			called = true
			gotArgs = args
			return nil
		},
	}
	err := cli.Dispatch([]string{"crap", "--coverprofile=cover.out"}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("crap handler not called")
	}
	if len(gotArgs) != 1 || gotArgs[0] != "--coverprofile=cover.out" {
		t.Fatalf("crap handler received unexpected args: %v", gotArgs)
	}
}

func TestDispatchEmptyArgs(t *testing.T) {
	cfg := cli.Config{
		Start:  func(_ []string) error { return nil },
		Notify: func(_ []string) error { return nil },
		Log:    func(_ []string) error { return nil },
	}
	err := cli.Dispatch([]string{}, cfg)
	if err == nil {
		t.Fatal("expected usage error")
	}
}

func TestDispatchUnknown(t *testing.T) {
	cfg := cli.Config{
		Start:  func(_ []string) error { return nil },
		Notify: func(_ []string) error { return nil },
		Log:    func(_ []string) error { return nil },
	}
	err := cli.Dispatch([]string{"unknown"}, cfg)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}
