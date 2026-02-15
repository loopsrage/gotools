package searcher

import (
	"golangApp/lib/jsonContainers/container"
	"strings"
)

// KeySearcher implements ContainerSearcher. The Key is the name attribute of a json object
// Results will be the *Container's who's value's type is map[string]interface{} and the Key is present
type KeySearcher struct {
	Key     string
	Results []*container.Container
}

// Current is called by Dfs to provide the current iterations *Container
func (o *KeySearcher) Current(cnt *container.Container) {
	switch v := cnt.Value().(type) {
	case map[string]interface{}:
		if _, ok := v[o.Key]; ok {
			o.Results = append(o.Results, cnt)
		}
	}
}

type ValueSearcher struct {
	results map[interface{}]interface{}
}

// Current is called by Dfs to provide the current iterations *Container
func (o *ValueSearcher) Current(cnt *container.Container) {
	switch v := cnt.Value().(type) {
	case map[string]interface{}:
		if o.results == nil {
			o.results = make(map[interface{}]interface{})
		}

		for key, value := range v {
			delim := string([]byte(cnt.Path())[4])
			o.results[strings.TrimPrefix(cnt.Path()[6:], delim)+delim+key] = value
		}
	}
}

// Dfs performs a depth first search on the provided *Container.Children and passes the current *Container
// into ContainerSearcher.Current
func Dfs(current *container.Container, searcher ContainerSearcher) (output []*container.Container) {
	if current == nil {
		return output
	}

	childs, err := current.Children()
	if err != nil {
		return nil
	}

	for _, v := range childs {
		output = append(output, Dfs(v, searcher)...)
	}

	searcher.Current(current)
	return output
}

func Flatten(cont *container.Container) (results map[interface{}]interface{}) {
	vs := &ValueSearcher{}
	Dfs(cont, vs)
	return vs.results
}

func FindObjectsWithKey(cont *container.Container, key string) (results map[string]*container.Container) {
	results = make(map[string]*container.Container)
	s := &KeySearcher{Key: key}
	Dfs(cont, s)
	for _, res := range s.Results {
		results[res.Path()] = res
	}
	return results
}

func FindObjectsWithKeys(start *container.Container, keys ...string) (results map[string]map[string]*container.Container) {
	results = make(map[string]map[string]*container.Container)
	for _, v := range keys {
		results[v] = FindObjectsWithKey(start, v)
	}
	return results
}
