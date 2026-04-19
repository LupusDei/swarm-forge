Feature: SwarmForge CRAP subcommand

  The `swarmforge crap` subcommand is the enforcement mechanism for
  Constitution Rule 4 (complexity and CRAP limits). It walks a Go
  source tree, computes cyclomatic complexity (CC) for every
  top-level function, intersects per-function line ranges with a
  supplied coverprofile to obtain a coverage fraction, and emits the
  CRAP score defined as:

      CRAP(m) = CC(m)^2 * (1 - cov(m))^3 + CC(m)

  Scores are binned into bands: low risk (< 5), moderate (< 30),
  and high risk (>= 30). The Constitution caps every function at
  CC <= 4 and CRAP < 30; this subcommand surfaces violations so
  the Simplicity Enforcer agent can act on them.

  The tool uses only the Go standard library. It does NOT generate
  coverprofiles itself — callers supply one produced by
  `go test -coverprofile=<path> ./...`.

  Background:
    Given the CLI binary name is "swarmforge"
    And the subcommand under test is "crap"
    And the tool is restricted to the Go standard library

  # ── CLI surface ─────────────────────────────────────────────────

  Scenario: crap subcommand is dispatched by the CLI
    Given the CLI receives arguments "crap" "--coverprofile=cover.out"
    Then the crap handler is invoked with arguments "--coverprofile=cover.out"

  Scenario: --coverprofile is required
    Given the user runs "swarmforge crap" with no --coverprofile flag
    Then the command exits with code 2
    And standard error contains "coverprofile"

  Scenario: --root defaults to the current working directory
    Given the user runs "swarmforge crap --coverprofile=cover.out" with no --root flag
    Then the analyzer walks the current working directory

  Scenario: --root selects an explicit analysis root
    Given the user runs "swarmforge crap --coverprofile=cover.out --root=./internal/crap"
    Then the analyzer walks "./internal/crap"

  Scenario: default thresholds are CC 4 and CRAP 30
    Given the user runs "swarmforge crap --coverprofile=cover.out" with no threshold flags
    Then the effective CC threshold is 4
    And the effective CRAP threshold is 30

  Scenario: threshold flags override defaults
    Given the user runs "swarmforge crap --coverprofile=cover.out --threshold-cc=8 --threshold-crap=50"
    Then the effective CC threshold is 8
    And the effective CRAP threshold is 50

  Scenario: I/O errors exit with code 2
    Given the user runs "swarmforge crap --coverprofile=does-not-exist.out"
    Then the command exits with code 2
    And standard error contains the path "does-not-exist.out"

  # ── Cyclomatic complexity counting ──────────────────────────────

  Scenario: a function with no branches has complexity 1
    Given a Go source file containing a top-level function with no control flow
    When the analyzer computes cyclomatic complexity
    Then the reported complexity is 1

  Scenario: each if statement adds one to complexity
    Given a Go function containing two top-level "if" statements
    When the analyzer computes cyclomatic complexity
    Then the reported complexity is 3

  Scenario: for and range loops each add one to complexity
    Given a Go function containing one "for" loop and one "for ... range" loop
    When the analyzer computes cyclomatic complexity
    Then the reported complexity is 3

  Scenario: each case clause in a switch adds one to complexity
    Given a Go function containing a switch statement with three case clauses
    When the analyzer computes cyclomatic complexity
    Then the reported complexity is 4

  Scenario: each case in a select adds one to complexity
    Given a Go function containing a select statement with two cases
    When the analyzer computes cyclomatic complexity
    Then the reported complexity is 3

  Scenario: each "&&" and "||" operator adds one to complexity
    Given a Go function whose body is "if a && b || c { return }"
    When the analyzer computes cyclomatic complexity
    Then the reported complexity is 4

  Scenario: anonymous function complexity rolls up into the enclosing top-level function
    Given a top-level function whose body contains one "if" statement
    And a closure inside that same function whose body contains two "if" statements
    When the analyzer computes cyclomatic complexity
    Then exactly one row is reported for the file
    And the reported complexity for the enclosing function is 4

  # ── File filtering ──────────────────────────────────────────────

  Scenario: _test.go files are excluded from analysis
    Given the analysis root contains "foo.go" and "foo_test.go"
    When the analyzer walks the source tree
    Then "foo.go" is parsed
    And "foo_test.go" is skipped

  Scenario: files under vendor/ are excluded from analysis
    Given the analysis root contains "vendor/x/y.go" and "internal/z.go"
    When the analyzer walks the source tree
    Then "internal/z.go" is parsed
    And "vendor/x/y.go" is skipped

  Scenario: files under hidden directories are excluded from analysis
    Given the analysis root contains ".git/a.go" and ".claude/b.go" and "pkg/c.go"
    When the analyzer walks the source tree
    Then "pkg/c.go" is parsed
    And ".git/a.go" is skipped
    And ".claude/b.go" is skipped

  Scenario: non-Go files are ignored
    Given the analysis root contains "README.md" and "main.go"
    When the analyzer walks the source tree
    Then "main.go" is parsed
    And "README.md" is skipped

  # ── Coverage intersection ───────────────────────────────────────

  Scenario: coverage fraction equals covered statements over total statements in overlapping blocks
    Given a Go function spanning lines 10 to 20 in "pkg/x.go"
    And a coverprofile with a block "pkg/x.go:10.2,15.3 4 1" (4 statements, hit)
    And a coverprofile with a block "pkg/x.go:16.2,20.3 4 0" (4 statements, not hit)
    When the analyzer computes coverage for the function
    Then the reported coverage fraction is 0.5

  Scenario: a function with no matching coverage blocks is treated as 0 coverage
    Given a Go function in "pkg/y.go" with no coverprofile blocks in that file
    When the analyzer computes coverage for the function
    Then the reported coverage fraction is 0.0

  Scenario: coverprofile mode line is tolerated
    Given a coverprofile whose first line is "mode: set"
    When the analyzer parses the coverprofile
    Then parsing succeeds and the mode line does not become a block

  Scenario: coverage blocks are matched to functions by file path and line range
    Given two functions "A" (lines 1..5) and "B" (lines 10..15) in "pkg/z.go"
    And a coverprofile with a block "pkg/z.go:10.1,15.1 3 1"
    When the analyzer computes coverage
    Then function "A" has coverage 0.0
    And function "B" has coverage 1.0

  # ── CRAP formula and bands ──────────────────────────────────────

  Scenario Outline: CRAP score is computed from CC and coverage
    Given a function with complexity <cc> and coverage <cov>
    When the analyzer computes the CRAP score
    Then the CRAP score rounds to <crap>
    And the band is "<band>"

    Examples:
      | cc | cov  | crap  | band     |
      | 1  | 1.00 | 1.00  | low      |
      | 2  | 0.50 | 2.50  | low      |
      | 4  | 1.00 | 4.00  | low      |
      | 4  | 0.00 | 20.00 | moderate |
      | 5  | 1.00 | 5.00  | moderate |
      | 5  | 0.00 | 30.00 | high     |
      | 10 | 0.50 | 22.50 | moderate |
      | 10 | 0.00 | 110.00| high     |

  # ── Text output (default) ───────────────────────────────────────

  Scenario: text output is a table sorted by CRAP descending then CC descending
    Given functions with these (file:line, name, CC, cov, CRAP) rows:
      | file      | line | name | cc | cov | crap  |
      | a.go      |   3  | A    |  2 | 1.0 |  2.0  |
      | b.go      |   7  | B    | 10 | 0.0 | 110.0 |
      | c.go      |  12  | C    |  5 | 0.5 |  5.625|
      | d.go      |  40  | D    |  5 | 1.0 |  5.0  |
    When the analyzer renders the text report with default thresholds
    Then the rows appear in this order: "B", "C", "D", "A"

  Scenario: text output includes required columns
    Given a report row for file "x.go" line 3 function "F" with CC 5, coverage 0.40, CRAP 12.48
    When the analyzer renders the text report
    Then the output contains "x.go:3"
    And the output contains "F"
    And the output contains the CC value "5"
    And the output contains a coverage percentage "40.0%"
    And the output contains the CRAP value "12.48"
    And the output contains the band "moderate"

  Scenario: violations column lists every threshold that a row exceeds
    Given default thresholds CC=4 and CRAP=30
    And a function with CC 6 and CRAP 12
    When the analyzer renders the text report
    Then the violations column for that row contains "cc"
    And the violations column for that row does not contain "crap"

    Given a function with CC 10 and CRAP 110
    When the analyzer renders the text report
    Then the violations column for that row contains "cc"
    And the violations column for that row contains "crap"

    Given a function with CC 3 and CRAP 4
    When the analyzer renders the text report
    Then the violations column for that row is empty

  # ── JSON output ─────────────────────────────────────────────────

  Scenario: --json emits a JSON array of row objects
    Given the user runs the analyzer with --json
    When the analyzer renders the report
    Then the output is valid JSON
    And the output is a JSON array
    And each array element has the fields "file", "line", "function", "cc", "coverage", "crap", "band", "violations"
    And "coverage" is a fraction between 0 and 1
    And "violations" is an array of strings

  Scenario: JSON rows are ordered by CRAP descending then CC descending
    Given functions with CRAP scores 110, 5.625, 5.0, and 2.0
    When the analyzer renders the JSON report
    Then the array elements appear in CRAP-descending order

  # ── Exit codes ──────────────────────────────────────────────────

  Scenario: clean run exits with code 0
    Given an analysis in which every function satisfies CC <= threshold-cc and CRAP < threshold-crap
    When the command finishes
    Then the exit code is 0

  Scenario: any violation exits with code 1
    Given an analysis in which at least one function has CC > threshold-cc
    When the command finishes
    Then the exit code is 1

    Given an analysis in which at least one function has CRAP >= threshold-crap
    When the command finishes
    Then the exit code is 1

  Scenario: usage errors exit with code 2
    Given the user supplies an unknown flag to the crap subcommand
    When the command finishes
    Then the exit code is 2
    And standard error contains a usage hint

  # ── Self-application ────────────────────────────────────────────

  Scenario: pointing the tool at its own source reports cleanly
    Given the SwarmForge repository at the current working directory
    And a coverprofile produced by running "go test -coverprofile=cover.out ./..." on this repository
    When the user runs "swarmforge crap --coverprofile=cover.out"
    Then the exit code is 0
    And no row in the report violates the default thresholds
