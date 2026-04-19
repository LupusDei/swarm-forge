package crap

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Row is a single function entry in the report.
type Row struct {
	File       string
	Line       int
	Function   string
	CC         int
	Coverage   float64
	CRAP       float64
	Band       string
	Violations []string
}

// Thresholds bounds the acceptable CC and CRAP per function.
type Thresholds struct {
	CC   int
	CRAP float64
}

// Config drives a single analysis run.
type Config struct {
	Root         string
	Coverprofile string
	JSON         bool
	Thresholds   Thresholds
	Stdout       io.Writer
	Stderr       io.Writer
}

// Analyze walks cfg.Root, parses every eligible Go source file, and
// returns one Row per top-level function or method, sorted by CRAP
// descending then CC descending.
func Analyze(cfg Config) ([]Row, error) {
	blocks, files, rs, err := loadAnalysisInputs(cfg)
	if err != nil {
		return nil, err
	}
	rows, err := buildRows(files, blocks, cfg.Thresholds, rs)
	if err != nil {
		return nil, err
	}
	sortRows(rows)
	return rows, nil
}

func loadAnalysisInputs(cfg Config) ([]Block, []string, *pathResolver, error) {
	blocks, err := loadBlocks(cfg.Coverprofile)
	if err != nil {
		return nil, nil, nil, err
	}
	rs, err := newResolver(cfg.Root)
	if err != nil {
		return nil, nil, nil, err
	}
	files, err := collectGoFiles(cfg.Root)
	if err != nil {
		return nil, nil, nil, err
	}
	return blocks, files, rs, nil
}

func loadBlocks(path string) ([]Block, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseCoverprofile(f)
}

func collectGoFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		return walkVisit(path, d, err, root, &files)
	})
	return files, err
}

func walkVisit(path string, d fs.DirEntry, err error, root string, files *[]string) error {
	if err != nil {
		return err
	}
	if d.IsDir() {
		return dirVisit(path, d, root)
	}
	if isGoSource(d.Name()) {
		*files = append(*files, path)
	}
	return nil
}

func dirVisit(path string, d fs.DirEntry, root string) error {
	if path == root {
		return nil
	}
	if isExcludedDir(d.Name()) {
		return filepath.SkipDir
	}
	return nil
}

func isExcludedDir(name string) bool {
	if name == "vendor" {
		return true
	}
	return strings.HasPrefix(name, ".")
}

func isGoSource(name string) bool {
	if !strings.HasSuffix(name, ".go") {
		return false
	}
	return !strings.HasSuffix(name, "_test.go")
}

func buildRows(files []string, blocks []Block, th Thresholds, rs *pathResolver) ([]Row, error) {
	var rows []Row
	for _, f := range files {
		rs2, err := rowsForFile(f, blocks, th, rs)
		if err != nil {
			return nil, err
		}
		rows = append(rows, rs2...)
	}
	return rows, nil
}

func rowsForFile(path string, blocks []Block, th Thresholds, rs *pathResolver) ([]Row, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	coveragePath := rs.blockPath(path)
	var rows []Row
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			rows = append(rows, rowForFunc(path, coveragePath, fset, fn, blocks, th))
		}
	}
	return rows, nil
}

func rowForFunc(displayPath, covPath string, fset *token.FileSet,
	fn *ast.FuncDecl, blocks []Block, th Thresholds) Row {
	sl := fset.Position(fn.Pos()).Line
	el := fset.Position(fn.End()).Line
	cc := ComplexityOfFunc(fn)
	cov := CoverageForRange(blocks, covPath, sl, el)
	crap := CrapScore(cc, cov)
	return Row{
		File: displayPath, Line: sl, Function: fn.Name.Name,
		CC: cc, Coverage: cov, CRAP: crap, Band: Band(crap),
		Violations: violationsFor(cc, crap, th),
	}
}

func violationsFor(cc int, crap float64, th Thresholds) []string {
	var v []string
	if cc > th.CC {
		v = append(v, "cc")
	}
	if crap >= th.CRAP {
		v = append(v, "crap")
	}
	return v
}

func sortRows(rows []Row) {
	sort.SliceStable(rows, func(i, j int) bool {
		return rowLess(rows[i], rows[j])
	})
}

func rowLess(a, b Row) bool {
	if a.CRAP != b.CRAP {
		return a.CRAP > b.CRAP
	}
	return a.CC > b.CC
}
