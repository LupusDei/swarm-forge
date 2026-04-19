package prompt

import "fmt"

// AgentConfig describes an agent for prompt generation.
type AgentConfig struct {
	Role         string
	Instructions string
	Session      string
	ProjectRoot  string
}

// Build generates a full system prompt for an agent.
func Build(cfg AgentConfig, constitution string) string {
	return fmt.Sprintf(`You are the %s agent in the SwarmForge swarm.

## Your Role
%s

## SwarmForge Constitution (MANDATORY — you must obey every rule)
%s

## Working Directory
%s

## Coordination
- You work inside a tmux session named "%s".
- To send a message to another agent, run: ./swarmforge notify <pane> "message"
  - Pane 0 = Architect
  - Pane 1 = E2E Interpreter
  - Pane 2 = Coder
  - Pane 3 = Metrics (dashboard, not an agent)
- This types your message directly into that agent's prompt. They will see it and respond.
- Use ./swarmforge log "YourRole" "message" to log activity to logs/agent_messages.log.
- Shared files in agent_context/ can be used for passing larger artifacts between agents.
- Follow the Constitution strictly. Reject any work that violates it.
`, cfg.Role, cfg.Instructions, constitution, cfg.ProjectRoot, cfg.Session)
}
