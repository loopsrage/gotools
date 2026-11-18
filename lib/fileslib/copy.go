package fileslib

import (
	"errors"
	"fmt"
	"io"
	"os"
)

func Copyfile(source, dest string) (err error) {
	src, err := os.Open(source)
	if err != nil {
		return errors.New(fmt.Sprintf("Error opening source file: %v", err))
	}
	defer src.Close()

	dst, err := os.Create(dest)
	if err != nil {
		return errors.New(fmt.Sprintf("Error creating destination file: %v", err))
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		return errors.New(fmt.Sprintf("Error copying file contents: %v", err))
	}
	return nil
}
