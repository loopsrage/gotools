package settings_test

import (
	"bytes"
	"testing"

	"golangApp/lib/settings"
)

func TestAddAndLoadAppSettings(t *testing.T) {
	settings.Add("testapp", map[string]interface{}{
		"testapp": map[string]interface{}{
			"name":  "TestApp",
			"port":  8080,
			"debug": true,
		},
	})

	apps, err := settings.Apps()
	if err != nil {
		t.Fatalf("Apps() error: %v", err)
	}
	found := false
	for _, a := range apps {
		if a == "testapp" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("testapp not found in Apps()")
	}

	loaded := settings.LoadAppSettings("testapp")
	if loaded == nil {
		t.Fatal("LoadAppSettings('testapp') returned nil")
	}
	appMap, ok := loaded["testapp"].(map[string]interface{})
	if !ok {
		t.Fatal("LoadAppSettings() did not return map[string]interface{}")
	}
	if appMap["name"] != "TestApp" {
		t.Fatalf("appMap['name'] = %v, want %v", appMap["name"], "TestApp")
	}
}

func TestLoadAppSettingHelpers(t *testing.T) {
	settings.Add("helperapp", map[string]interface{}{
		"helperapp": map[string]interface{}{
			"strval":  "hello",
			"intval":  int64(42),
			"boolval": true,
			"strlist": []string{"a", "b", "c"},
		},
	})

	if got := settings.LoadAppSettingString("helperapp", "strval"); got != "hello" {
		t.Fatalf("LoadAppSettingString('helperapp', 'strval') = %q, want %q", got, "hello")
	}
	if got := settings.LoadAppSettingInt64("helperapp", "intval"); got != 42 {
		t.Fatalf("LoadAppSettingInt64('helperapp', 'intval') = %d, want %d", got, 42)
	}
	if got := settings.LoadAppSettingBool("helperapp", "boolval"); got != true {
		t.Fatalf("LoadAppSettingBool('helperapp', 'boolval') = %v, want %v", got, true)
	}
	list := settings.LoadAppSettingStrings("helperapp", "strlist")
	if len(list) != 3 || list[0] != "a" || list[1] != "b" || list[2] != "c" {
		t.Fatalf("LoadAppSettingStrings('helperapp', 'strlist') = %v, want [a b c]", list)
	}
}

func TestLoadAppSettingMissing(t *testing.T) {
	if got := settings.LoadAppSettingString("missingapp", "setting"); got != "" {
		t.Fatalf("LoadAppSettingString('missingapp', 'setting') = %q, want %q", got, "")
	}
	if got := settings.LoadAppSettingInt64("missingapp", "setting"); got != 0 {
		t.Fatalf("LoadAppSettingInt64('missingapp', 'setting') = %d, want %d", got, 0)
	}
}

func TestYaml(t *testing.T) {
	settings.Add("yamltest", map[string]interface{}{
		"yamltest": map[string]interface{}{
			"key": "value",
		},
	})
	yamlBytes, err := settings.Yaml("yamltest")
	if err != nil {
		t.Fatalf("Yaml() error: %v", err)
	}
	if len(yamlBytes) == 0 {
		t.Fatal("Yaml() returned empty bytes")
	}
}

func TestEnv(t *testing.T) {
	settings.Add("envtest", map[string]interface{}{
		"envtest": map[string]interface{}{
			"KEY": "VALUE",
		},
	})
	envBytes, err := settings.Env("envtest")
	if err != nil {
		t.Fatalf("Env() error: %v", err)
	}
	if len(envBytes) == 0 {
		t.Fatal("Env() returned empty bytes")
	}
}

func TestWriteAllYaml(t *testing.T) {
	settings.Add("writeapp1", map[string]interface{}{
		"writeapp1": map[string]interface{}{"k": "v1"},
	})
	settings.Add("writeapp2", map[string]interface{}{
		"writeapp2": map[string]interface{}{"k": "v2"},
	})

	var buf bytes.Buffer
	size, err := settings.WriteAllYaml(&buf)
	if err != nil {
		t.Fatalf("WriteAllYaml() error: %v", err)
	}
	if size == 0 {
		t.Fatal("WriteAllYaml() returned size 0")
	}
	if buf.Len() == 0 {
		t.Fatal("WriteAllYaml() wrote nothing to buffer")
	}
}

var _ = bytes.MinRead
