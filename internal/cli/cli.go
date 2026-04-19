package cli

import "fmt"

// Handler is called when a subcommand is matched.
type Handler func(args []string) error

// Config holds the registered subcommand handlers.
type Config struct {
	Start  Handler
	Notify Handler
	Log    Handler
	Crap   Handler
}

// Dispatch routes CLI arguments to the appropriate handler.
func Dispatch(args []string, cfg Config) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: swarmforge <start|notify|log|crap> [args...]")
	}
	handler, ok := handlerMap(cfg)[args[0]]
	if !ok {
		return fmt.Errorf("unknown command: %s", args[0])
	}
	return handler(args[1:])
}

func handlerMap(cfg Config) map[string]Handler {
	return map[string]Handler{
		"start":  cfg.Start,
		"notify": cfg.Notify,
		"log":    cfg.Log,
		"crap":   cfg.Crap,
	}
}
