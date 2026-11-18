package featureSettings_test

import (
	"testing"

	"github.com/loopsrage/gotools/lib/featureSettings"
)

type sampleStruct struct {
	Host string
	Port int
}

func TestConfigAttributesRoundTrip(t *testing.T) {
	cfg := featureSettings.NewConfig()
	src := &sampleStruct{Host: "127.0.0.1", Port: 8081}
	cfg.AggregateAttributes(src)

	all := cfg.AllAttributes()
	if len(all) != 2 {
		t.Fatalf("AllAttributes len = %d, want 2", len(all))
	}

	dst := &sampleStruct{}
	cfg.ReadAttributes(dst)
	if dst.Host != "127.0.0.1" || dst.Port != 8081 {
		t.Fatalf("ReadAttributes = %+v, want %+v", dst, src)
	}
}
