package jsonIndexer

import (
	"encoding/json"
	"golangApp/lib/index"
	"golangApp/lib/jsonContainers/container"
	"golangApp/lib/jsonContainers/containerTreeBuilder"
	"golangApp/lib/jsonContainers/searcher"
	"io"
	"regexp"
	"strings"
)

type JsonIndexer struct {
	idx  *index.Index
	cidx *index.Index
}

const conta = "containers"
const values = "values"

func (s *JsonIndexer) StoreSettings(cont *container.Container) {
	rv := searcher.Flatten(cont)
	for key, val := range rv {
		s.idx.StoreInIndex(values, key.(string), val)
	}
	s.cidx.StoreInIndex(conta, cont, struct{}{})
}

func (s *JsonIndexer) ReadJson(reader io.Reader) error {
	b, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	x := make(map[string]interface{})
	err = json.Unmarshal(b, &x)
	if err != nil {
		return err
	}

	cont := containerTreeBuilder.BuildContainerTree(nil, []string{}, '.', x)
	s.StoreSettings(cont)
	return nil
}

func (s *JsonIndexer) Setting(appName, path string) (interface{}, error) {
	return s.idx.LoadFromIndex(appName, path)
}

func (s *JsonIndexer) Int64(appName, path string) int64 {
	x, _ := s.Setting(appName, path)
	switch x := x.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	default:
		return -1
	}
}

func (s *JsonIndexer) String(appName, path string) string {
	x, _ := s.Setting(appName, path)
	switch x := x.(type) {
	case string:
		return x
	default:
		return ""
	}
}

func (s *JsonIndexer) Strings(appName, path string) []string {
	x, _ := s.Setting(appName, path)
	switch x := x.(type) {
	case []string:
		return x
	default:
		return nil
	}
}

func (s *JsonIndexer) Bool(appName, path string) bool {
	x, _ := s.Setting(appName, path)
	switch x := x.(type) {
	case bool:
		return x
	default:
		return false
	}
}

func (s *JsonIndexer) FindValues(reg *regexp.Regexp) (out map[string]interface{}, err error) {
	out = make(map[string]interface{})
	err = s.idx.RangeIndex(values, func(key, value interface{}) {
		if reg.MatchString(key.(string)) {
			prts := strings.Split(key.(string), ".")
			out[prts[len(prts)-1]] = value
		}
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func NewJsonIndexer() *JsonIndexer {
	return &JsonIndexer{
		idx:  index.NewIndex(),
		cidx: index.NewIndex(),
	}
}
