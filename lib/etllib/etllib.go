package etllib

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/loopsrage/gotools/lib/fuzzy"
)

// ValidationReport summarizes CSV structural quality and detected anomalies.
type ValidationReport struct {
	Valid         bool     `json:"valid"`
	Columns       []string `json:"columns"`
	RowCount      int      `json:"row_count"`
	EmptyRows     int      `json:"empty_rows"`
	MismatchedLen int      `json:"mismatched_column_rows"`
	Errors        []string `json:"errors,omitempty"`
}

// ValidateCSV parses r as CSV and checks header presence and column counts.
func ValidateCSV(r io.Reader, delimiter rune) (ValidationReport, error) {
	cr := csv.NewReader(r)
	if delimiter != 0 {
		cr.Comma = delimiter
	}
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return ValidationReport{Valid: false, Errors: []string{err.Error()}}, err
	}
	cols := make([]string, len(header))
	for i, h := range header {
		cols[i] = strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))
	}
	rep := ValidationReport{Valid: true, Columns: cols}
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			rep.Valid = false
			rep.Errors = append(rep.Errors, err.Error())
			continue
		}
		allEmpty := true
		for _, c := range rec {
			if strings.TrimSpace(c) != "" {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			rep.EmptyRows++
			continue
		}
		if len(rec) != len(cols) {
			rep.MismatchedLen++
		}
		rep.RowCount++
	}
	if len(cols) == 0 {
		rep.Valid = false
		rep.Errors = append(rep.Errors, "empty CSV header")
	}
	return rep, nil
}

// TransformCSVToJSON converts CSV rows into typed JSON objects (coercing booleans, ints, floats).
func TransformCSVToJSON(r io.Reader, delimiter rune) ([]map[string]any, []string, error) {
	cr := csv.NewReader(r)
	if delimiter != 0 {
		cr.Comma = delimiter
	}
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return nil, nil, err
	}
	cols := make([]string, len(header))
	for i, h := range header {
		cols[i] = strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))
	}
	var rows []map[string]any
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, cols, err
		}
		row := make(map[string]any, len(cols))
		nonEmpty := false
		for i, col := range cols {
			var raw string
			if i < len(rec) {
				raw = strings.TrimSpace(rec[i])
			}
			if raw != "" {
				nonEmpty = true
			}
			row[col] = coerceScalar(raw)
		}
		if nonEmpty {
			rows = append(rows, row)
		}
	}
	return rows, cols, nil
}

func coerceScalar(s string) any {
	if s == "" {
		return nil
	}
	low := strings.ToLower(s)
	if low == "true" {
		return true
	}
	if low == "false" {
		return false
	}
	if iv, err := strconv.ParseInt(s, 10, 64); err == nil {
		return iv
	}
	if fv, err := strconv.ParseFloat(s, 64); err == nil {
		return fv
	}
	return s
}

// InferJSONSchema generates a JSON Schema draft-07 object descriptor from row records.
func InferJSONSchema(rows []map[string]any) map[string]any {
	typesByCol := make(map[string]map[string]bool)
	for _, r := range rows {
		for k, v := range r {
			if _, ok := typesByCol[k]; !ok {
				typesByCol[k] = make(map[string]bool)
			}
			if v == nil {
				typesByCol[k]["null"] = true
				continue
			}
			switch v.(type) {
			case bool:
				typesByCol[k]["boolean"] = true
			case int, int64:
				typesByCol[k]["integer"] = true
			case float64:
				typesByCol[k]["number"] = true
			default:
				typesByCol[k]["string"] = true
			}
		}
	}
	props := make(map[string]any, len(typesByCol))
	for col, seen := range typesByCol {
		t := "string"
		if seen["string"] {
			t = "string"
		} else if seen["number"] {
			t = "number"
		} else if seen["integer"] {
			t = "integer"
		} else if seen["boolean"] {
			t = "boolean"
		}
		if seen["null"] {
			props[col] = map[string]any{"type": []string{t, "null"}}
		} else {
			props[col] = map[string]any{"type": t}
		}
	}
	return map[string]any{
		"$schema":    "http://json-schema.org/draft-07/schema#",
		"type":       "object",
		"properties": props,
	}
}

// DeduplicateRows removes near-duplicate rows whose keyField Indel similarity >= threshold (0..100).
func DeduplicateRows(rows []map[string]any, keyField string, threshold float64) ([]map[string]any, int) {
	if threshold <= 0 {
		threshold = 90.0
	}
	var kept []map[string]any
	var keptKeys []string
	removed := 0
	for _, r := range rows {
		val := strings.TrimSpace(fmt.Sprint(r[keyField]))
		dup := false
		if val != "" && val != "<nil>" {
			for _, prev := range keptKeys {
				if fuzzy.Ratio(val, prev) >= threshold {
					dup = true
					break
				}
			}
		}
		if dup {
			removed++
			continue
		}
		kept = append(kept, r)
		keptKeys = append(keptKeys, val)
	}
	return kept, removed
}
