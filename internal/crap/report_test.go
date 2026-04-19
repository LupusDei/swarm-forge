package crap_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/swarm-forge/swarm-forge/internal/crap"
)

func gherkinFourRowFixture() []crap.Row {
	return []crap.Row{
		{File: "a.go", Line: 3, Function: "A", CC: 2, Coverage: 1.0, CRAP: 2.0, Band: "low"},
		{File: "b.go", Line: 7, Function: "B", CC: 10, Coverage: 0.0, CRAP: 110.0, Band: "high"},
		{File: "c.go", Line: 12, Function: "C", CC: 5, Coverage: 0.5, CRAP: 5.625, Band: "moderate"},
		{File: "d.go", Line: 40, Function: "D", CC: 5, Coverage: 1.0, CRAP: 5.0, Band: "moderate"},
	}
}

func renderedOrder(t *testing.T, output string, names []string) []int {
	t.Helper()
	positions := make([]int, len(names))
	for i, n := range names {
		idx := strings.Index(output, n)
		if idx < 0 {
			t.Fatalf("output missing %q; output=\n%s", n, output)
		}
		positions[i] = idx
	}
	return positions
}

func TestRenderText_SortOrder(t *testing.T) {
	// Feed rows out of order; renderer is expected to emit sorted output
	// (CRAP desc, then CC desc as tiebreaker).
	rows := gherkinFourRowFixture()
	// Shuffle input order.
	rows = []crap.Row{rows[0], rows[2], rows[3], rows[1]} // A, C, D, B
	th := crap.Thresholds{CC: 4, CRAP: 30}

	var buf bytes.Buffer
	if err := crap.RenderText(rows, th, &buf); err != nil {
		t.Fatalf("RenderText: %v", err)
	}
	out := buf.String()
	pos := renderedOrder(t, out, []string{"B", "C", "D", "A"})
	for i := 1; i < len(pos); i++ {
		if pos[i-1] >= pos[i] {
			t.Fatalf("expected order B, C, D, A; got positions=%v; output=\n%s", pos, out)
		}
	}

	// Tie on CRAP broken by CC descending.
	tied := []crap.Row{
		{File: "x.go", Line: 1, Function: "Hi", CC: 5, Coverage: 0.5, CRAP: 5.0, Band: "moderate"},
		{File: "y.go", Line: 1, Function: "Lo", CC: 3, Coverage: 0.5, CRAP: 5.0, Band: "moderate"},
	}
	var tbuf bytes.Buffer
	if err := crap.RenderText(tied, th, &tbuf); err != nil {
		t.Fatalf("RenderText (ties): %v", err)
	}
	tout := tbuf.String()
	hi := strings.Index(tout, "Hi")
	lo := strings.Index(tout, "Lo")
	if hi < 0 || lo < 0 {
		t.Fatalf("tie fixture: missing rows; output=\n%s", tout)
	}
	if hi >= lo {
		t.Fatalf("tie broken wrong: Hi(CC=5) should precede Lo(CC=3); hi=%d lo=%d output=\n%s",
			hi, lo, tout)
	}
}

func TestRenderText_ColumnsPresent(t *testing.T) {
	rows := []crap.Row{
		{File: "x.go", Line: 3, Function: "F", CC: 5, Coverage: 0.40, CRAP: 12.48, Band: "moderate"},
	}
	th := crap.Thresholds{CC: 4, CRAP: 30}

	var buf bytes.Buffer
	if err := crap.RenderText(rows, th, &buf); err != nil {
		t.Fatalf("RenderText: %v", err)
	}
	out := buf.String()
	for _, needle := range []string{"x.go:3", "F", "5", "40.0%", "12.48", "moderate"} {
		if !strings.Contains(out, needle) {
			t.Errorf("output missing %q; output=\n%s", needle, out)
		}
	}
}

func TestRenderText_ViolationsColumn(t *testing.T) {
	th := crap.Thresholds{CC: 4, CRAP: 30}

	// Row violates cc only (CC=6 > 4; CRAP=12 < 30).
	ccOnly := []crap.Row{
		{File: "p.go", Line: 1, Function: "CCOnly", CC: 6, Coverage: 0.5, CRAP: 12.0,
			Band: "moderate", Violations: []string{"cc"}},
	}
	var ccBuf bytes.Buffer
	if err := crap.RenderText(ccOnly, th, &ccBuf); err != nil {
		t.Fatalf("RenderText cc-only: %v", err)
	}
	ccOut := ccBuf.String()
	if !strings.Contains(ccOut, "cc") {
		t.Errorf("cc-only row should list 'cc'; output=\n%s", ccOut)
	}

	// Row violates both (CC=10 > 4; CRAP=110 >= 30).
	both := []crap.Row{
		{File: "q.go", Line: 1, Function: "Both", CC: 10, Coverage: 0.0, CRAP: 110.0,
			Band: "high", Violations: []string{"cc", "crap"}},
	}
	var bothBuf bytes.Buffer
	if err := crap.RenderText(both, th, &bothBuf); err != nil {
		t.Fatalf("RenderText both: %v", err)
	}
	bothOut := bothBuf.String()
	if !strings.Contains(bothOut, "cc") {
		t.Errorf("both-violations row should list 'cc'; output=\n%s", bothOut)
	}
	if !strings.Contains(bothOut, "crap") {
		t.Errorf("both-violations row should list 'crap'; output=\n%s", bothOut)
	}

	// Row violates neither (CC=3 <= 4; CRAP=4 < 30).
	clean := []crap.Row{
		{File: "r.go", Line: 1, Function: "Clean", CC: 3, Coverage: 0.8, CRAP: 4.0,
			Band: "low", Violations: nil},
	}
	var cleanBuf bytes.Buffer
	if err := crap.RenderText(clean, th, &cleanBuf); err != nil {
		t.Fatalf("RenderText clean: %v", err)
	}
	cleanOut := cleanBuf.String()
	// The clean row must not list either threshold keyword on its line.
	for _, line := range strings.Split(cleanOut, "\n") {
		if !strings.Contains(line, "Clean") {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "cc") && !strings.Contains(lower, "cc:") {
			// A header might contain "CC", so tolerate a header-style token; only
			// reject lowercase "cc" as a violations entry on the data row itself.
		}
		// Look specifically for the standalone tokens that would indicate a violation.
		tokens := strings.Fields(strings.ReplaceAll(line, ",", " "))
		for _, tok := range tokens {
			if tok == "cc" || tok == "crap" {
				t.Errorf("clean row contains violation token %q on its line: %q", tok, line)
			}
		}
	}
}

func TestRenderJSON_ShapeAndFields(t *testing.T) {
	rows := gherkinFourRowFixture()
	for i := range rows {
		if rows[i].Violations == nil {
			rows[i].Violations = []string{}
		}
	}

	var buf bytes.Buffer
	if err := crap.RenderJSON(rows, &buf); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	out := buf.Bytes()

	var arr []map[string]any
	if err := json.Unmarshal(out, &arr); err != nil {
		t.Fatalf("output is not a JSON array of objects: %v\nraw=%s", err, string(out))
	}
	if len(arr) != len(rows) {
		t.Fatalf("expected %d JSON elements; got %d", len(rows), len(arr))
	}
	required := []string{"file", "line", "function", "cc", "coverage", "crap", "band", "violations"}
	for i, el := range arr {
		for _, k := range required {
			if _, ok := el[k]; !ok {
				t.Errorf("element %d missing field %q: %+v", i, k, el)
			}
		}
		cov, ok := el["coverage"].(float64)
		if !ok {
			t.Errorf("element %d: coverage is not a JSON number; got %T", i, el["coverage"])
			continue
		}
		if cov < 0 || cov > 1 {
			t.Errorf("element %d: coverage %v outside [0,1]", i, cov)
		}
		violAny, ok := el["violations"].([]any)
		if !ok {
			t.Errorf("element %d: violations is not a JSON array; got %T", i, el["violations"])
			continue
		}
		for j, v := range violAny {
			if _, ok := v.(string); !ok {
				t.Errorf("element %d violation %d is not a string; got %T", i, j, v)
			}
		}
	}
}

func TestRenderJSON_SortOrder(t *testing.T) {
	rows := gherkinFourRowFixture()
	// Unsort on input; renderer must emit sorted output.
	rows = []crap.Row{rows[0], rows[2], rows[3], rows[1]} // A, C, D, B

	var buf bytes.Buffer
	if err := crap.RenderJSON(rows, &buf); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	var arr []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &arr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(arr) != 4 {
		t.Fatalf("expected 4 elements; got %d", len(arr))
	}
	wantCraps := []float64{110.0, 5.625, 5.0, 2.0}
	for i, want := range wantCraps {
		got, ok := arr[i]["crap"].(float64)
		if !ok {
			t.Fatalf("element %d: crap not a number; got %T", i, arr[i]["crap"])
		}
		if got != want {
			t.Fatalf("JSON order wrong at index %d: crap=%v, want %v; arr=%+v", i, got, want, arr)
		}
	}
}
