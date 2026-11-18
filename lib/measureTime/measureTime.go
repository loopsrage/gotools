package measureTime

import (
	"fmt"
	"time"
)

func MeasureTime(start *time.Time) {
	fmt.Println(fmt.Sprintf("[%s] Request Duration %f", time.Now().Format(time.RFC3339), time.Since(*start).Seconds()))
}
