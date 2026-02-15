package timelib

import (
	"fmt"
	"testing"
	"time"
)

type testTargetTime struct{}

func (t *testTargetTime) Window(current time.Time) {
	// TODO implement me
	fmt.Println(current)
}

func TestFromStartToEnd(t *testing.T) {
	t.Run("FromStartToEnd", func(t *testing.T) {
		FromStartToEnd(&testTargetTime{}, time.Now(), time.Now().Add(-(30 * time.Hour * 24)), -24*time.Hour)
	})
}

func TestFromEndToStart(t *testing.T) {
	t.Run("FromEndToStart", func(t *testing.T) {
		FromEndToStart(&testTargetTime{}, time.Now(), time.Now().Add(-(30 * time.Hour * 24)), -24*time.Hour)
	})
}
