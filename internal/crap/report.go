package crap

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// RenderText writes a sorted, tab-separated table of rows to w.
func RenderText(rows []Row, _ Thresholds, w io.Writer) error {
	sortRows(rows)
	for _, r := range rows {
		if _, err := fmt.Fprintln(w, formatTextRow(r)); err != nil {
			return err
		}
	}
	return nil
}

func formatTextRow(r Row) string {
	return fmt.Sprintf("%s:%d\t%s\t%d\t%.1f%%\t%.2f\t%s\t%s",
		r.File, r.Line, r.Function, r.CC,
		r.Coverage*100, r.CRAP, r.Band,
		strings.Join(r.Violations, ","))
}

// RenderJSON writes a sorted JSON array of row objects to w.
func RenderJSON(rows []Row, w io.Writer) error {
	sortRows(rows)
	out := make([]jsonRow, len(rows))
	for i, r := range rows {
		out[i] = toJSONRow(r)
	}
	return json.NewEncoder(w).Encode(out)
}

type jsonRow struct {
	File       string   `json:"file"`
	Line       int      `json:"line"`
	Function   string   `json:"function"`
	CC         int      `json:"cc"`
	Coverage   float64  `json:"coverage"`
	CRAP       float64  `json:"crap"`
	Band       string   `json:"band"`
	Violations []string `json:"violations"`
}

func toJSONRow(r Row) jsonRow {
	v := r.Violations
	if v == nil {
		v = []string{}
	}
	return jsonRow{
		File: r.File, Line: r.Line, Function: r.Function,
		CC: r.CC, Coverage: r.Coverage, CRAP: r.CRAP,
		Band: r.Band, Violations: v,
	}
}
