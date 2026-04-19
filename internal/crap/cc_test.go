package crap_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/swarm-forge/swarm-forge/internal/crap"
)

func parseFunc(t *testing.T, src string) *ast.FuncDecl {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			return fn
		}
	}
	t.Fatal("no func decl in source")
	return nil
}

func TestComplexityOfFunc(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "baseline_no_branches",
			src: `package p
func F() { x := 1; _ = x }`,
			want: 1,
		},
		{
			name: "two_if_statements",
			src: `package p
func F(a, b int) {
	if a > 0 { _ = a }
	if b > 0 { _ = b }
}`,
			want: 3,
		},
		{
			name: "for_and_range",
			src: `package p
func F(xs []int) {
	for i := 0; i < 3; i++ { _ = i }
	for _, v := range xs { _ = v }
}`,
			want: 3,
		},
		{
			name: "switch_three_cases",
			src: `package p
func F(x int) {
	switch x {
	case 1:
		_ = x
	case 2:
		_ = x
	case 3:
		_ = x
	}
}`,
			want: 4,
		},
		{
			name: "select_two_cases",
			src: `package p
func F(a, b chan int) {
	select {
	case <-a:
	case <-b:
	}
}`,
			want: 3,
		},
		{
			name: "mixed_and_or_in_if",
			src: `package p
func F(a, b, c bool) {
	if a && b || c { return }
}`,
			want: 4,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseFunc(t, tc.src)
			got := crap.ComplexityOfFunc(fn)
			if got != tc.want {
				t.Fatalf("CC = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestComplexityOfFunc_FoldsClosures(t *testing.T) {
	src := `package p
func F(a, b, c int) {
	if a > 0 {
		fn := func() {
			if b > 0 { _ = b }
			if c > 0 { _ = c }
		}
		fn()
	}
}`
	fn := parseFunc(t, src)
	got := crap.ComplexityOfFunc(fn)
	if got != 4 {
		t.Fatalf("expected folded CC=4 (1 base + 1 outer if + 2 closure ifs); got %d", got)
	}
}
