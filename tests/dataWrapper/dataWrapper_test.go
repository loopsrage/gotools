package dataWrapper_test

import (
	"encoding/json"
	"testing"

	"github.com/loopsrage/gotools/lib/dataWrapper"
)

func TestDataWrapperJSON(t *testing.T) {
	orig := dataWrapper.DataWrapper{
		Data: []byte("payload"),
		Name: "doc.pdf",
	}
	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded dataWrapper.DataWrapper
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Name != orig.Name || string(decoded.Data) != string(orig.Data) {
		t.Fatalf("decoded = %+v, want %+v", decoded, orig)
	}
}
