package util

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type CountFile struct {
	countData *sync.Map
	Total     int64
}

func (c *CountFile) Count(name string) int64 {
	x, _ := c.countData.Load(name)
	switch x := x.(type) {
	case int64:
		return x
	default:
		return 0
	}
}

func (c *CountFile) Increment(name string, delta int64) {
	current := c.Count(name+".") + delta
	c.countData.Store(name+".", current)
	c.Total += delta
}

func (c *CountFile) FindMissing() map[string]int64 {
	missing := make(map[string]int64)
	c.countData.Range(func(key, value interface{}) bool {
		switch key := key.(type) {
		case string:
			switch value := value.(type) {
			case int64:
				if value != 4 {
					missing[key] = value
				}
			}
		}
		return true
	})
	fmt.Println(missing)
	return missing
}

var reg = regexp.MustCompile("\\.(.*)")

func (c *CountFile) CountFiles(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		delt := 1
		if info.Size() == 0 {
			delt = 1
		}
		path = reg.ReplaceAllString(path, "")
		name := path[strings.LastIndex(path, "/"):]
		c.Increment(name, int64(delt))
		return nil
	})
}

func NewCountFiles() *CountFile {
	return &CountFile{
		countData: &sync.Map{},
	}
}

type DeleteAction struct{}

func (d *DeleteAction) Action(on interface{}) {
	switch on := on.(type) {
	case string:
		err := DeleteFiles(on)
		if err != nil {
			fmt.Println(err)
			return
		}
	}
}

func DeleteFiles(prefix string) error {
	matches, err := filepath.Glob(fmt.Sprintf("/home/jarek/tess/tesstrain/data/AmazonEmber-ground-truth%s*", prefix))
	if err != nil {
		return err
	}
	for _, match := range matches {
		if err := os.RemoveAll(match); err != nil { // Use os.RemoveAll for files/directories
			fmt.Printf("Failed to remove %s: %v\n", match, err)
			return err
		}
	}
	return nil
}

func DeleteAllFiles() error {
	err := os.RemoveAll("/home/jarek/tess/tesstrain/data/AmazonEmber-ground-truth")
	if err != nil {
		return err
	}

	err = os.MkdirAll("/home/jarek/tess/tesstrain/data/AmazonEmber-ground-truth", 0755)
	if err != nil {
		return err
	}
	return nil
}

type MakeAction struct {
	Ctx context.Context
}

var mar = regexp.MustCompile("At iteration(.*)")

func (m *MakeAction) MakeTraining() error {

	cmd := []string{
		"training", "MODEL_NAME=AmazonEmber",
		"START_MODEL=eng",
		"TESSDATA=/home/jarek/tess/tesstrain",
		"PSM=8",
		"MAX_ITERATIONS=5170000",
	}

	venvBin := "/home/jarek/tess/tesstrain/.venv/bin/" // Use \\Scripts on Windows

	// Get the current system PATH and prepend the venv's bin directory
	newPath := fmt.Sprintf("%s:%s", venvBin, os.Getenv("PATH")) // Use ; on Windows

	out := exec.CommandContext(m.Ctx, "make", cmd...)
	out.Dir = "/home/jarek/tess/tesstrain"
	// Set the environment for the command
	out.Env = os.Environ()                     // Inherit existing env
	out.Env = append(out.Env, "PATH="+newPath) // Add the new PATH

	opb, err := out.CombinedOutput()
	if err != nil {
		return errors.New(fmt.Sprintf("[%s] Command failed: %s. Command was %s %s", time.Now().Format(time.RFC3339), err, "make", strings.Join(cmd, " ")))
	}
	fmt.Println(mar.FindString(string(opb)))
	//fmt.Println(string(opb))
	return nil
}

// map[/B0EZTVYLT:1 /B0M5QNWAP:1 /B0RRYHDDL:1 /B0S8D1YVC:1]
