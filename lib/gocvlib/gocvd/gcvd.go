package gocvd

import (
	"gocv.io/x/gocv"
)

func DebugWindow(img *gocv.Mat, win *gocv.Window) {
	win = gocv.NewWindow("Debug")
	defer win.Close()

	err := win.IMShow(*img)
	if err != nil {
		panic(err)
	}

	win.WaitKey(0)

	gocv.WaitKey(1)
}
