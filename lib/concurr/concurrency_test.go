package concurr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"testing"
	"time"
)

type A struct{}

func (a *A) Action(on interface{}) {
	http.DefaultClient.Timeout = 10 * time.Second
	resp, err := http.Get("http://localhost:8080/Plants/List?search=&page=1&limit=100&sortModel=W10%3D&filterModel=e30%3D&rowGroupCols=W10%3D&groupKeys=W10%3D&valueCols=W10%3D&pivotCols=W10%3D")
	if err != nil {
		fmt.Println(err)
		return
	}

	var b []byte
	b, err = io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return
	}

	var x = make(map[string]interface{})
	err = json.Unmarshal(b, &x)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(on)
}

// BenchmarkActionOn tests the performance of processing a large number of items concurrently.
func BenchmarkActionOn(b *testing.B) {
	const numItems = 1000

	concurrencyLevels := []int{1, 10, 50, 100, 200, 500}
	if runtime.NumCPU() > 1 {
		concurrencyLevels = append(concurrencyLevels, runtime.NumCPU())
	}

	for _, max := range concurrencyLevels {
		b.Run(fmt.Sprintf("max_in_flight=%d", max), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Setup for each benchmark iteration
				items := make([]interface{}, numItems)
				for j := 0; j < numItems; j++ {
					items[j] = j
				}

				config := &Config{}
				action := &A{}

				ActionOn(context.Background(), config, action, items...)

			}
		})
	}
}
