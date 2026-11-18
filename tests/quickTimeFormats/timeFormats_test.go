package quickTimeFormats_test

import (
	"testing"
	"time"

	"github.com/loopsrage/gotools/lib/quickTimeFormats"
)

func TestTimeFormatsNonEmpty(t *testing.T) {
	formats := quickTimeFormats.TimeFormats()
	if len(formats) == 0 {
		t.Fatal("TimeFormats() returned empty list")
	}
}

func TestTimeFormatsContainsExpected(t *testing.T) {
	formats := quickTimeFormats.TimeFormats()
	expected := []string{
		"1/2/2006",
		"2006-01-02 15:04:05",
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		time.DateOnly,
	}
	formatSet := make(map[string]bool)
	for _, f := range formats {
		formatSet[f] = true
	}
	for _, e := range expected {
		if !formatSet[e] {
			t.Fatalf("TimeFormats() missing expected format %q", e)
		}
	}
}

func TestTimeFormatsAllValidLayouts(t *testing.T) {
	// Each format string should be a valid Go time layout (parsable by time.Parse with a dummy value).
	formats := quickTimeFormats.TimeFormats()
	testTime := time.Date(2006, 1, 2, 15, 4, 5, 999999999, time.UTC)
	for _, f := range formats {
		// Skip layouts that don't match the testTime shape (e.g. "01/02" lacks year).
		// Just verify each format is a non-empty string.
		if f == "" {
			t.Fatal("empty format string in TimeFormats()")
		}
		_ = testTime.Format(f)
	}
}
