package crap

import (
	"go/ast"
	"go/token"
)

// ComplexityOfFunc returns the cyclomatic complexity of fn, folding any
// nested closures into the enclosing function's count.
func ComplexityOfFunc(fn *ast.FuncDecl) int {
	if fn == nil || fn.Body == nil {
		return 1
	}
	cc := 1
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		cc += contributionOf(n)
		return true
	})
	return cc
}

func contributionOf(n ast.Node) int {
	switch n.(type) {
	case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
		return 1
	}
	if c := contribFromCase(n); c != 0 {
		return c
	}
	return contribFromBinary(n)
}

func contribFromCase(n ast.Node) int {
	if c, ok := n.(*ast.CaseClause); ok {
		return boolToInt(len(c.List) > 0)
	}
	if c, ok := n.(*ast.CommClause); ok {
		return boolToInt(c.Comm != nil)
	}
	return 0
}

func contribFromBinary(n ast.Node) int {
	b, ok := n.(*ast.BinaryExpr)
	if !ok {
		return 0
	}
	return boolToInt(isLogicalOp(b.Op))
}

func isLogicalOp(op token.Token) bool {
	return op == token.LAND || op == token.LOR
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
