// Package crap implements the `swarmforge crap` subcommand. It walks a
// Go source tree, computes cyclomatic complexity (CC) per top-level
// function, intersects function ranges with a caller-supplied
// coverprofile, and reports the CRAP score:
//
//	CRAP(m) = CC(m)^2 * (1 - cov(m))^3 + CC(m)
//
// The package is the enforcement engine for Constitution Rule 3 (CC)
// and Rule 4 (CRAP).
package crap
