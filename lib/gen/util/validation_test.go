package util

import (
	"github.com/loopsrage/gotools/lib/concurr"
	"context"
	"fmt"
	"testing"
)

func TestCountFiles(t *testing.T) {
	t.Run("test FIles", func(t *testing.T) {
		nc := NewCountFiles()
		err := nc.CountFiles("/home/jarek/tess/tesstrain/data/AmazonEmber-ground-truth")
		if err != nil {
			t.Errorf("Error in CountFiles: %v", err)
		}
		fmt.Println(nc.Total, nc.FindMissing())
		da := &DeleteAction{}
		var x []interface{}
		for k, _ := range nc.FindMissing() {
			x = append(x, k)
		}
		concurr.ActionOn(context.Background(), concurr.NewConfig(24), da, x...)
	})
}
