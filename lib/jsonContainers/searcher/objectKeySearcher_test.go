package searcher

import (
	"encoding/json"
	"fmt"
	"github.com/loopsrage/gotools/lib/jsonContainers/container"
	"github.com/loopsrage/gotools/lib/jsonContainers/containerTreeBuilder"
	"os"
	"testing"
)

func TestFindObjectsWithKeys(t *testing.T) {

	x, err := os.ReadFile("/home/jarek/GolandProjects/uploader-server-lib-v3/lib/jsonContainers/testFiles/example.json")
	if err != nil {
		t.Fatal(err)
	}

	var m = make(map[string]interface{})
	err = json.Unmarshal(x, &m)
	if err != nil {
		t.Fatal(err)
	}

	type args struct {
		cont    *container.Container
		results map[string]*KeySearcher
		keys    []string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "",
			args: args{
				cont: containerTreeBuilder.BuildContainerTree(nil, nil, '_', m),
				keys: []string{"index", "op"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := FindObjectsWithKey(tt.args.cont, "index")
			fmt.Printf("%+v\n", res)
		})
	}
}
