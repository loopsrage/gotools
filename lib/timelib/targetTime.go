package timelib

import (
	"math"
	"time"
)

type TargetTime interface {
	Window(current time.Time)
}

func FromStartToEnd(targetTime TargetTime, start, end time.Time, windowSize time.Duration) {
	for start.Before(end) {
		targetTime.Window(start)
		start = start.Add(windowSize)
	}
}

func FromEndToStart(targetTime TargetTime, start, end time.Time, windowSize time.Duration) {
	for end.Before(start) {
		targetTime.Window(end)
		end = end.Add(time.Duration(math.Abs(float64(windowSize))))
	}
}
