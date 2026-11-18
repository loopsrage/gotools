package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/loopsrage/gotools/lib/index"
	"github.com/loopsrage/gotools/lib/jsonContainers/container"
	"github.com/loopsrage/gotools/lib/jsonContainers/containerTreeBuilder"
	"github.com/loopsrage/gotools/lib/jsonContainers/searcher"
	"github.com/loopsrage/gotools/lib/reflectStruct"
	"io"
	"os"
	"regexp"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

const apps = "applications"

var envFileBytes *atomic.Value
var appIndex = index.NewIndex()
var settingsContainer *container.Container

func environmentFileBytes() []byte {
	if envFileBytes == nil {
		return nil
	}

	switch e := envFileBytes.Load().(type) {
	case []byte:
		return e
	default:
		return nil
	}
}

func Yaml(appName string) ([]byte, error) {
	b, err := yaml.Marshal(LoadAppSettings(appName))
	if err != nil {
		return nil, err
	}
	return b, nil
}

func Env(appName string) ([]byte, error) {

	md, err := reflectStruct.MarshallMap(LoadAppSettings(appName))
	if err != nil {
		return nil, err
	}

	root := containerTreeBuilder.BuildContainerTree(nil, nil, '_', md)
	buff := bytes.NewBuffer(nil)
	rv := searcher.Flatten(root)
	for key, value := range rv {
		switch value.(type) {
		case map[string]interface{}, []interface{}:
			continue
		}
		buff.WriteString(fmt.Sprintf("%s=%v\n", key, value))
	}
	return buff.Bytes(), nil
}

func LoadAppSettings(appName string) map[string]interface{} {
	x, _ := appIndex.LoadFromIndex(apps, appName)
	switch x := x.(type) {
	case map[string]interface{}:
		return x
	default:
		return nil
	}
}

func LoadAppSettingString(appName, setting string) string {
	x, _ := LoadAppSetting(appName, setting).(string)
	return x
}

func LoadAppSettingStrings(appName, setting string) []string {
	switch x := LoadAppSetting(appName, setting).(type) {
	case []string:
		return x
	case []interface{}:
		var out []string
		for _, v := range x {
			switch v := v.(type) {
			case string:
				out = append(out, v)
			}
		}
		return out
	default:
		return nil
	}
}

func LoadAppSettingBool(appName, setting string) bool {
	x, _ := LoadAppSetting(appName, setting).(bool)
	return x
}

func LoadAppSettingInt64(appName, setting string) int64 {
	x, _ := LoadAppSetting(appName, setting).(int64)
	return x
}

func LoadAppSetting(appName, setting string) interface{} {
	appSettings := LoadAppSettings(appName)
	if appSettings == nil {
		return nil
	}
	appMap, ok := appSettings[appName].(map[string]interface{})
	if !ok {
		return nil
	}
	return appMap[setting]
}

func JsonUnmarshalSettingsTo(appName string, appConfig interface{}) error {
	bb := environmentFileBytes()
	if bb != nil {
		var xf = make(map[string]interface{})
		err := json.Unmarshal(bb, &xf)
		if err != nil {
			return err
		}

		reflectStruct.FindFields(appConfig, xf)
		return nil
	}

	set := LoadAppSettings(appName)
	if set == nil {
		return errors.New(fmt.Sprintf("settings not loaded for %s", appName))
	}

	b, err := json.Marshal(set[appName])
	if err != nil {
		return err
	}

	return json.Unmarshal(b, appConfig)
}

func Add(appName string, config interface{}) {
	appIndex.StoreInIndex(apps, appName, config)
}

func Apps() (registeredApps []string, err error) {
	err = appIndex.RangeIndex(apps, func(key, value interface{}) {
		switch key := key.(type) {
		case string:
			registeredApps = append(registeredApps, key)
		}
	})
	if err != nil {
		return nil, err
	}
	return registeredApps, nil
}

func WriteAllYaml(writer io.Writer) (size int64, err error) {
	appList, err := Apps()
	if err != nil {
		return 0, err
	}

	for _, v := range appList {
		yb, err := Yaml(v)
		if err != nil {
			return 0, err
		}

		wc, err := writer.Write(yb)
		if err != nil {
			return 0, err
		}
		size += int64(wc)
	}
	return size, nil
}

func WriteAllEnv(writer io.Writer) (size int64, err error) {
	appList, err := Apps()
	if err != nil {
		return 0, err
	}

	for _, v := range appList {
		yb, err := Env(v)
		if err != nil {
			return 0, err
		}

		wc, err := writer.Write(yb)
		if err != nil {
			return 0, err
		}
		size += int64(wc)
	}
	return size, nil
}

var envFile = regexp.MustCompile("\\.env$")
var yamlFile = regexp.MustCompile("(.*)(\\.yaml|\\.yml)")

func RestoreFromFile(path string) (err error) {
	var b []byte
	b, err = os.ReadFile(path)
	if err != nil {
		return err
	}

	if yamlFile.MatchString(path) {
		var x = make(map[string]interface{})
		err = yaml.Unmarshal(b, &x)
		if err != nil {
			return err
		}

		for appName, v := range x {
			app := map[string]interface{}{appName: v}
			settingsContainer = containerTreeBuilder.BuildContainerTree(settingsContainer, nil, '_', app)
			Add(appName, app)
		}
		return nil
	}

	if envFile.MatchString(path) {
		envFileBytes = &atomic.Value{}
		lines := bytes.Split(b, []byte("\n"))
		appData := make(map[string]interface{})
		for _, line := range lines {
			ln := bytes.Split(line, []byte("="))
			if len(ln) != 2 {
				continue
			}
			appData[string(ln[0])] = string(ln[1])
		}

		var vb []byte
		vb, err = json.Marshal(appData)
		if err != nil {
			return err
		}
		envFileBytes.Store(vb)
		return nil
	}

	return errors.New("unable to restore from file")
}

func RootSettings() *container.Container {
	return settingsContainer
}

func init() {
	ef := os.Getenv("ENVIRONMENT_FILE")
	if ef != "" {
		err := RestoreFromFile(ef)
		if err != nil {
			panic(err)
		}
		return
	}

	if dir, err := os.Getwd(); err == nil {
		defaultEnv := dir + "/env.yaml"
		if _, statErr := os.Stat(defaultEnv); statErr == nil {
			if err := RestoreFromFile(defaultEnv); err != nil {
				panic(err)
			}
		}
	}
}
