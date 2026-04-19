package prompt_test

import (
	"strings"
	"testing"

	"github.com/swarm-forge/swarm-forge/internal/prompt"
)

func TestArchitectInstructionsNotEmpty(t *testing.T) {
	if prompt.ArchitectInstructions == "" {
		t.Fatal("ArchitectInstructions is empty")
	}
}

func TestCoderInstructionsNotEmpty(t *testing.T) {
	if prompt.CoderInstructions == "" {
		t.Fatal("CoderInstructions is empty")
	}
}

func TestE2EInterpreterInstructionsNotEmpty(t *testing.T) {
	if prompt.E2EInterpreterInstructions == "" {
		t.Fatal("E2EInterpreterInstructions is empty")
	}
}

func TestBuildContainsRole(t *testing.T) {
	cfg := prompt.AgentConfig{
		Role:         "Architect",
		Instructions: prompt.ArchitectInstructions,
		Session:      "swarmforge",
		ProjectRoot:  "/project",
	}
	result := prompt.Build(cfg, "Constitution text")
	if !strings.Contains(result, "You are the Architect agent") {
		t.Fatalf("prompt missing role header: %s", result)
	}
}

func TestBuildContainsConstitution(t *testing.T) {
	cfg := prompt.AgentConfig{
		Role:         "Coder",
		Instructions: prompt.CoderInstructions,
		Session:      "swarmforge",
		ProjectRoot:  "/project",
	}
	result := prompt.Build(cfg, "Rule 1: TDD")
	if !strings.Contains(result, "Rule 1: TDD") {
		t.Fatalf("prompt missing constitution")
	}
}

func TestBuildContainsCoordination(t *testing.T) {
	cfg := prompt.AgentConfig{
		Role:         "Coder",
		Instructions: prompt.CoderInstructions,
		Session:      "swarmforge",
		ProjectRoot:  "/project",
	}
	result := prompt.Build(cfg, "")
	if !strings.Contains(result, "./swarmforge notify") {
		t.Fatal("prompt missing ./swarmforge notify")
	}
	if !strings.Contains(result, "./swarmforge log") {
		t.Fatal("prompt missing ./swarmforge log")
	}
	if !strings.Contains(result, "agent_context/") {
		t.Fatal("prompt missing agent_context/")
	}
	if !strings.Contains(result, "Pane 0 = Architect") {
		t.Fatal("prompt missing pane layout")
	}
}

// C1 red tests — E2E-Interpreter owns coverage, not passing.
func TestE2EInterpreterPromptScopesToCoverageOnly(t *testing.T) {
	cfg := prompt.AgentConfig{
		Role:         "E2E-Interpreter",
		Instructions: prompt.E2EInterpreterInstructions,
		Session:      "swarmforge",
		ProjectRoot:  "/project",
	}
	result := prompt.Build(cfg, "Constitution content")

	mustContain := []string{
		"cover every Gherkin scenario with a failing end-to-end test",
		"hand off the failing E2E tests to the Coder",
	}
	for _, s := range mustContain {
		if !strings.Contains(result, s) {
			t.Errorf("E2E-Interpreter prompt missing required phrase %q; got:\n%s", s, result)
		}
	}

	forbidden := "Ensure all Gherkin scenarios pass before any feature is marked complete"
	if strings.Contains(result, forbidden) {
		t.Errorf("E2E-Interpreter prompt still contains forbidden phrase %q; got:\n%s", forbidden, result)
	}
}

// C1 red test — Coder owns making E2E tests pass.
func TestCoderPromptStatesResponsibilityForPassingE2ETests(t *testing.T) {
	cfg := prompt.AgentConfig{
		Role:         "Coder",
		Instructions: prompt.CoderInstructions,
		Session:      "swarmforge",
		ProjectRoot:  "/project",
	}
	result := prompt.Build(cfg, "Constitution content")

	mustContain := []string{
		"receive failing end-to-end tests from the E2E Interpreter",
		"implement the feature until every E2E test passes",
	}
	for _, s := range mustContain {
		if !strings.Contains(result, s) {
			t.Errorf("Coder prompt missing required phrase %q; got:\n%s", s, result)
		}
	}
}
