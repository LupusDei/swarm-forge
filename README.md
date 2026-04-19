# SwarmForge

**A disciplined tmux-based agent orchestration platform that turns swarms of AI agents into reliable, professional software engineers.**

## Intent

SwarmForge exists to solve the core problem of agentic development: **chaos**.

Left unchecked, AI agents produce code quickly but often without discipline, leading to brittle, untested, hard-to-maintain software. SwarmForge changes that by embedding **strict professional craftsmanship** directly into the platform.

It enforces four foundational clean code disciplines — plus static linting — as an unbreakable **Constitution**. Every agent in the swarm must obey these rules on every task. The result is fast, scalable, and genuinely high-quality software produced reliably at swarm speed.

SwarmForge turns raw AI coding power into **disciplined, trustworthy engineering**.

## What SwarmForge Does

SwarmForge is a lightweight, tmux-based orchestration layer that:

- Spawns and coordinates a **swarm of specialized AI agents** (Architect, Coder, TDD Guardian, E2E Interpreter, Simplicity Enforcer, Mutation Hunter, Linter Guardian, etc.)
- Manages real-time collaboration between agents through named tmux panes and shared file system
- Enforces the **SwarmForge Constitution** on every change:
  1. **TDD** – Tests first, always (Red → Green → Refactor)
  2. **E2E Gherkin Tests** – Features described in business language and automatically interpreted into executable end-to-end tests
  3. **Cyclomatic Complexity + CRAP Score** – Keeps every method simple and low-risk
  4. **Mutation Testing** – Agents deliberately break the code to ensure tests are meaningful
  5. **Linter Enforcement** – Zero warnings, consistent style and quality
- Provides live visibility into the swarm’s progress through tmux panes and a growing metrics dashboard
- Builds production-grade, maintainable code while staying true to clean code principles

## Core Features

- **Constitution-Driven Development** — The rules are not suggestions. Agents that violate them are blocked.
- **Self-Hosted & Lightweight** — Runs locally in tmux. No heavy dependencies or cloud costs for the core.
- **Dogfooding** — SwarmForge uses its own swarm to extend and improve itself.
- **Observable Swarm** — Watch multiple agents reason, write tests, run mutations, and refactor in real time.
- **Scalable Foundation** — Designed as the base layer for future distributed/cloud swarms and an Electron GUI.

## How It Works (High Level)

1. Launch `./swarmforge.sh` — starts the tmux session with the swarm.
2. Give the swarm a task (via main Architect pane or feature Gherkin file).
3. Agents collaborate under the Constitution:
   - Requirements → Gherkin scenarios
   - Gherkin → E2E tests
   - TDD cycle for implementation
   - Mutation testing + complexity checks
   - Linter validation
4. Only clean, tested, mutation-killed, low-complexity code is accepted.

The swarm refuses to ship anything that doesn’t meet the standards.

## Who Is SwarmForge For?

- Developers who want to harness AI agents without sacrificing code quality
- Teams exploring agentic development practices
- Anyone tired of “AI wrote it” meaning “now I have to rewrite it”
- Clean Code enthusiasts who believe discipline still matters in the age of agents

## Getting Started

### Prerequisites

- Go 1.25+
- tmux
- watch
- [claude](https://github.com/anthropics/claude-code) CLI, signed in

### Build

```bash
git clone https://github.com/LupusDei/swarm-forge.git
cd swarm-forge
go build -o swarmforge ./cmd/swarmforge
```

### Start the swarm

```bash
./swarmforge start
```

This launches a tmux session named `swarmforge` with four panes — Architect, E2E-Interpreter, Coder, and a metrics tail on `logs/agent_messages.log`. Attach with `tmux attach -t swarmforge` if you detach.

### Other subcommands

```bash
./swarmforge notify <pane> "message"   # send a message to pane 0–3
./swarmforge log <role> "message"      # append a line to the agent log
```