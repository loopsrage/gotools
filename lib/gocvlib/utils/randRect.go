package utils

import (
	"crypto/rand"
	"image"
	r2 "math/rand"

	"gocv.io/x/gocv"
)

func RandomRect(img *gocv.Mat, point image.Point) image.Rectangle {
	targetWidth := img.Cols()
	targetHeight := img.Rows()

	maxX := point.X - targetWidth
	maxY := point.Y - targetHeight

	randX := r2.Intn(maxX + 1)
	randY := r2.Intn(maxY + 1)

	return image.Rectangle{
		Min: image.Point{X: randX, Y: randY},
		Max: image.Point{X: randX + targetWidth, Y: randY + targetHeight},
	}
}

func FullRect(img *gocv.Mat) image.Rectangle {
	return image.Rectangle{
		Min: image.Point{
			X: 0,
			Y: 0,
		},
		Max: image.Point{
			X: img.Cols(),
			Y: img.Rows(),
		},
	}
}

func rb() uint8 {
	var randomByte [1]byte // A slice of 1 byte is enough
	rand.Read(randomByte[:])
	return randomByte[0]
}

func RandColor() [3]uint8 {
	return [3]uint8{rb(), rb(), rb()}
}

func RandColorFloat64() [3]float64 {
	rc := RandColor()
	return [3]float64{float64(rc[0]), float64(rc[1]), float64(rc[2])}
}
