package crap

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Block is one entry from a Go coverprofile.
type Block struct {
	File      string
	StartLine int
	EndLine   int
	NumStmts  int
	Count     int
}

// ParseCoverprofile reads a Go coverprofile from r. The optional first
// `mode: <kind>` line is tolerated and not returned as a Block.
func ParseCoverprofile(r io.Reader) ([]Block, error) {
	var blocks []Block
	scanner := bufio.NewScanner(r)
	isFirst := true
	for scanner.Scan() {
		b, ok, err := parseScannedLine(scanner.Text(), isFirst)
		isFirst = false
		if err != nil {
			return nil, err
		}
		if ok {
			blocks = append(blocks, b)
		}
	}
	return blocks, scanner.Err()
}

func parseScannedLine(line string, isFirst bool) (Block, bool, error) {
	if shouldSkipLine(line, isFirst) {
		return Block{}, false, nil
	}
	b, err := parseBlockLine(line)
	if err != nil {
		return Block{}, false, err
	}
	return b, true, nil
}

func shouldSkipLine(line string, isFirst bool) bool {
	if line == "" {
		return true
	}
	return isFirst && strings.HasPrefix(line, "mode:")
}

func parseBlockLine(line string) (Block, error) {
	parts := strings.Fields(line)
	if len(parts) != 3 {
		return Block{}, fmt.Errorf("malformed coverprofile line: %q", line)
	}
	file, sl, el, err := parseRangeSpec(parts[0])
	if err != nil {
		return Block{}, err
	}
	nums, err := parseNums(parts[1], parts[2])
	if err != nil {
		return Block{}, fmt.Errorf("bad numbers in %q: %w", line, err)
	}
	return Block{
		File: file, StartLine: sl, EndLine: el,
		NumStmts: nums[0], Count: nums[1],
	}, nil
}

func parseNums(a, b string) ([2]int, error) {
	n1, err := strconv.Atoi(a)
	if err != nil {
		return [2]int{}, err
	}
	n2, err := strconv.Atoi(b)
	if err != nil {
		return [2]int{}, err
	}
	return [2]int{n1, n2}, nil
}

func parseRangeSpec(spec string) (string, int, int, error) {
	colon := strings.LastIndex(spec, ":")
	if colon < 0 {
		return "", 0, 0, fmt.Errorf("no colon in %q", spec)
	}
	sl, el, err := parseRangePair(spec[colon+1:])
	return spec[:colon], sl, el, err
}

func parseRangePair(rng string) (int, int, error) {
	comma := strings.Index(rng, ",")
	if comma < 0 {
		return 0, 0, fmt.Errorf("no comma in %q", rng)
	}
	sl, err := lineFromPart(rng[:comma])
	if err != nil {
		return 0, 0, err
	}
	el, err := lineFromPart(rng[comma+1:])
	if err != nil {
		return 0, 0, err
	}
	return sl, el, nil
}

func lineFromPart(part string) (int, error) {
	dot := strings.Index(part, ".")
	if dot < 0 {
		return strconv.Atoi(part)
	}
	return strconv.Atoi(part[:dot])
}

// CoverageForRange returns the fraction of statements in blocks that
// overlap [startLine, endLine] in file and were executed (Count > 0).
// Returns 0 when no blocks overlap.
func CoverageForRange(blocks []Block, file string, startLine, endLine int) float64 {
	total, covered := sumOverlapping(blocks, file, startLine, endLine)
	if total == 0 {
		return 0
	}
	return float64(covered) / float64(total)
}

func sumOverlapping(blocks []Block, file string, startLine, endLine int) (int, int) {
	var total, covered int
	for _, b := range blocks {
		if !overlaps(b, file, startLine, endLine) {
			continue
		}
		total += b.NumStmts
		if b.Count > 0 {
			covered += b.NumStmts
		}
	}
	return total, covered
}

func overlaps(b Block, file string, startLine, endLine int) bool {
	if b.File != file {
		return false
	}
	return b.StartLine <= endLine && b.EndLine >= startLine
}

// CrapScore returns CC^2 * (1 - cov)^3 + CC.
func CrapScore(cc int, cov float64) float64 {
	cf := float64(cc)
	return cf*cf*pow3(1-cov) + cf
}

func pow3(x float64) float64 {
	return x * x * x
}

// Band returns "low" (CRAP < 5), "moderate" (CRAP < 30), or "high".
func Band(crap float64) string {
	if crap < 5 {
		return "low"
	}
	if crap < 30 {
		return "moderate"
	}
	return "high"
}
