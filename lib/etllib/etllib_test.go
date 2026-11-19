package etllib

import (
	"strings"
	"testing"
)

func TestETLValidateTransformSchemaDedupe(t *testing.T) {
	csvData := "name,age,active\nAlice Smith,30,true\nAlice Smyth,30,true\nBob Jones,25,false\n"
	rep, err := ValidateCSV(strings.NewReader(csvData), ',')
	if err != nil || !rep.Valid || rep.RowCount != 3 {
		t.Fatalf("ValidateCSV: %+v err=%v", rep, err)
	}
	rows, cols, err := TransformCSVToJSON(strings.NewReader(csvData), ',')
	if err != nil || len(cols) != 3 || len(rows) != 3 {
		t.Fatalf("TransformCSVToJSON: rows=%d cols=%v err=%v", len(rows), cols, err)
	}
	if rows[0]["age"] != int64(30) || rows[0]["active"] != true {
		t.Fatalf("type coercion mismatch: %#v", rows[0])
	}
	schema := InferJSONSchema(rows)
	props := schema["properties"].(map[string]any)
	if props["age"].(map[string]any)["type"] != "integer" {
		t.Fatalf("schema mismatch: %#v", schema)
	}
	deduped, removed := DeduplicateRows(rows, "name", 85.0)
	if removed != 1 || len(deduped) != 2 {
		t.Fatalf("DeduplicateRows: kept=%d removed=%d", len(deduped), removed)
	}
}
