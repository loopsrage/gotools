package measureTime_test

import (
	"testing"
	"time"

	"github.com/loopsrage/gotools/lib/measureTime"
)

func TestMeasureTime(t *testing.T) {
	start := time.Now()
	measureTime.MeasureTime(&start)
}
