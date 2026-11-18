package containerTreeBuilder

import (
	"github.com/loopsrage/gotools/lib/jsonContainers/container"
	"strconv"
	"strings"
)

func BuildContainerTree(current *container.Container, path []string, pathDelim byte, start ...interface{}) *container.Container {
	if current == nil {
		if len(start) == 0 {
			start = []interface{}{map[string]interface{}{}}
		}
		current = container.NewContainer(nil, "", start[0])
		path = []string{"root"}
	}

	switch x := current.GetValue().(type) {
	case map[string]interface{}:
		for n, v := range x {
			switch v.(type) {
			case []interface{}, map[string]interface{}:
				path = append(path, n)
				jp := strings.Join(path, string(pathDelim))
				BuildContainerTree(current.NewObject(current, jp, v), path, pathDelim)
				path = path[:len(path)-1]
			}
		}
	case []interface{}:
		for i, v := range x {
			switch v.(type) {
			case []interface{}, map[string]interface{}:
				path = append(path, strconv.Itoa(i))
				jp := strings.Join(path, string(pathDelim))
				BuildContainerTree(current.NewObject(current, jp, v), path, pathDelim)
				path = path[:len(path)-1]
			}
		}
	}
	return current
}
