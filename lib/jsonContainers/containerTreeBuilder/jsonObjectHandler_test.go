package containerTreeBuilder

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestUnmarshalHelperQueue(t *testing.T) {

	x, err := os.ReadFile("/home/jarek/GolandProjects/uploader-server-lib-v3/lib/jsonContainers/testFiles/example.json")
	if err != nil {
		t.Fatal(err)
	}

	var m = make(map[string]interface{})
	err = json.Unmarshal(x, &m)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Testing json Containers", func(t *testing.T) {
		root := BuildContainerTree(nil, nil, '_', m)
		err = root.RangeValues(func(key string, value interface{}) {
			fmt.Printf("key: %s\n", key)
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}
