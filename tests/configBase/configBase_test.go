package configBase_test

import (
	"testing"

	"github.com/loopsrage/gotools/lib/configBase"
	"github.com/loopsrage/gotools/lib/settings"
)

type testFeature struct {
	Port int `json:"Port"`
}

func (f *testFeature) Source() string {
	return "TestFeatureBase"
}

func TestNewFeatureController(t *testing.T) {
	settings.Add("TestFeatureBase", map[string]interface{}{
		"TestFeatureBase": map[string]interface{}{
			"Enabled": true,
			"Port":    9090,
		},
	})

	feat := &testFeature{}
	fc := configBase.NewFeatureController(feat)
	defer fc.Close()

	if feat.Port != 9090 {
		t.Fatalf("feat.Port = %d, want 9090", feat.Port)
	}
	if fc.Attribute("Port") != 9090 {
		t.Fatalf("fc.Attribute('Port') = %v, want 9090", fc.Attribute("Port"))
	}
}
