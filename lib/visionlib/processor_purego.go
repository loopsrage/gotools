//go:build !gocv

package visionlib

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

type pureGoProcessor struct{}

// NewProcessor returns the active vision Processor implementation (pure-Go when built without -tags gocv).
func NewProcessor() Processor {
	return &pureGoProcessor{}
}

func (p *pureGoProcessor) NormalizeToPNG(raw []byte) (image.Image, []byte, int, int, error) {
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, 0, 0, err
	}
	b := img.Bounds()
	if format == "png" && b.Min.X == 0 && b.Min.Y == 0 {
		return img, raw, b.Dx(), b.Dy(), nil
	}
	norm := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(norm, norm.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(norm, norm.Bounds(), img, b.Min, draw.Over)
	pngBytes, err := EncodePNG(norm)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	return norm, pngBytes, b.Dx(), b.Dy(), nil
}

func (p *pureGoProcessor) SegmentRegions(pngBytes []byte, minArea, maxArea int) ([]byte, []image.Rectangle, error) {
	img, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, nil, errors.New("visionlib: empty image dimensions")
	}

	gray := toGrayGrid(img)
	blurred := gaussianBlur5x5(gray, w, h)
	fgMask := adaptiveForegroundMask(blurred, w, h, maxInt(15, minInt(w, h)/12|1), 10)
	bridged := morphDilateRect(fgMask, w, h, maxInt(6, w/60), maxInt(2, h/120))

	rawBoxes := connectedComponentBoxes(bridged, w, h)
	pageRect := image.Rect(0, 0, w, h)
	sorted := SortAndFilterBoxes(rawBoxes, pageRect, minArea, maxArea)

	for i := range sorted {
		padX := maxInt(4, sorted[i].Dx()/40)
		padY := maxInt(4, sorted[i].Dy()/20)
		sorted[i] = image.Rect(
			maxInt(0, sorted[i].Min.X-padX),
			maxInt(0, sorted[i].Min.Y-padY),
			minInt(w, sorted[i].Max.X+padX),
			minInt(h, sorted[i].Max.Y+padY),
		)
	}
	sorted = mergeOverlappingBoxes(sorted)

	if len(sorted) == 0 {
		sorted = []image.Rectangle{pageRect}
	}

	maskImg := maskToGrayImage(bridged, w, h, true)
	maskPNG, err := EncodePNG(maskImg)
	if err != nil {
		return nil, nil, err
	}
	return maskPNG, sorted, nil
}

func (p *pureGoProcessor) CropAndDenoise(pngBytes []byte, box image.Rectangle, preset DenoisePreset) ([]byte, []byte, error) {
	img, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, nil, err
	}
	fullBounds := image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy())
	cropRect := box.Intersect(fullBounds)
	if cropRect.Empty() {
		cropRect = fullBounds
	}

	cw, ch := cropRect.Dx(), cropRect.Dy()
	cropRGBA := image.NewRGBA(image.Rect(0, 0, cw, ch))
	draw.Draw(cropRGBA, cropRGBA.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(cropRGBA, cropRGBA.Bounds(), img, image.Pt(img.Bounds().Min.X+cropRect.Min.X, img.Bounds().Min.Y+cropRect.Min.Y), draw.Over)

	cropRawPNG, err := EncodePNG(cropRGBA)
	if err != nil {
		return nil, nil, err
	}

	if preset == PresetPassthrough {
		return cropRawPNG, cropRawPNG, nil
	}

	gray := toGrayGrid(cropRGBA)
	if medianLuminance(gray) < 110 {
		for i := range gray {
			gray[i] = 255 - gray[i]
		}
	}

	switch preset {
	case PresetWordIsolation:
		smoothed := gaussianBlur5x5(gray, cw, ch)
		fg := adaptiveForegroundMask(smoothed, cw, ch, maxInt(15, minInt(cw, ch)/10|1), 10)
		dilated := morphDilateRect(fg, cw, ch, 5, 2)
		outImg := maskToPaddedGrayImage(dilated, cw, ch, 8)
		denPNG, err := EncodePNG(outImg)
		return cropRawPNG, denPNG, err

	default:
		smoothed := gaussianBlur3x3(gray, cw, ch)
		win := maxInt(15, minInt(cw, ch)/8|1)
		fg := adaptiveForegroundMask(smoothed, cw, ch, win, 10)
		cleaned := despeckleIsolatedPixels(fg, cw, ch)
		outImg := maskToPaddedGrayImage(cleaned, cw, ch, 8)
		denPNG, err := EncodePNG(outImg)
		return cropRawPNG, denPNG, err
	}
}

func toGrayGrid(img image.Image) []uint8 {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a == 0 {
				out[y*w+x] = 255
				continue
			}
			alpha := float64(a) / 65535.0
			rf := (float64(r)/65535.0)*alpha + (1.0 - alpha)
			gf := (float64(g)/65535.0)*alpha + (1.0 - alpha)
			bf := (float64(bl)/65535.0)*alpha + (1.0 - alpha)
			lum := (0.299*rf + 0.587*gf + 0.114*bf) * 255.0
			if lum < 0 {
				lum = 0
			} else if lum > 255 {
				lum = 255
			}
			out[y*w+x] = uint8(lum + 0.5)
		}
	}
	return out
}

func medianLuminance(gray []uint8) uint8 {
	var hist [256]int
	for _, v := range gray {
		hist[v]++
	}
	target := len(gray) / 2
	acc := 0
	for i := 0; i < 256; i++ {
		acc += hist[i]
		if acc >= target {
			return uint8(i)
		}
	}
	return 255
}

func gaussianBlur3x3(src []uint8, w, h int) []uint8 {
	tmp := make([]uint16, w*h)
	out := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		row := y * w
		for x := 0; x < w; x++ {
			xm1 := maxInt(0, x-1)
			xp1 := minInt(w-1, x+1)
			tmp[row+x] = uint16(src[row+xm1]) + 2*uint16(src[row+x]) + uint16(src[row+xp1])
		}
	}
	for y := 0; y < h; y++ {
		ym1 := maxInt(0, y-1) * w
		yc := y * w
		yp1 := minInt(h-1, y+1) * w
		for x := 0; x < w; x++ {
			sum := tmp[ym1+x] + 2*tmp[yc+x] + tmp[yp1+x]
			out[yc+x] = uint8((sum + 8) >> 4)
		}
	}
	return out
}

func gaussianBlur5x5(src []uint8, w, h int) []uint8 {
	pass1 := gaussianBlur3x3(src, w, h)
	return gaussianBlur3x3(pass1, w, h)
}

func adaptiveForegroundMask(gray []uint8, w, h, windowSize, cOffset int) []bool {
	integral := make([]int64, (w+1)*(h+1))
	stride := w + 1
	for y := 1; y <= h; y++ {
		var rowSum int64
		for x := 1; x <= w; x++ {
			rowSum += int64(gray[(y-1)*w+(x-1)])
			integral[y*stride+x] = integral[(y-1)*stride+x] + rowSum
		}
	}

	half := windowSize / 2
	if half < 2 {
		half = 2
	}
	mask := make([]bool, w*h)
	for y := 0; y < h; y++ {
		y0 := maxInt(0, y-half)
		y1 := minInt(h, y+half+1)
		for x := 0; x < w; x++ {
			x0 := maxInt(0, x-half)
			x1 := minInt(w, x+half+1)
			area := int64((y1 - y0) * (x1 - x0))
			sum := integral[y1*stride+x1] - integral[y0*stride+x1] - integral[y1*stride+x0] + integral[y0*stride+x0]
			mean := int(sum / area)
			mask[y*w+x] = int(gray[y*w+x]) < (mean - cOffset)
		}
	}
	return mask
}

func despeckleIsolatedPixels(fg []bool, w, h int) []bool {
	if w < 3 || h < 3 {
		return fg
	}
	out := make([]bool, len(fg))
	copy(out, fg)
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			idx := y*w + x
			if !fg[idx] {
				continue
			}
			neighbors := 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					if fg[(y+dy)*w+(x+dx)] {
						neighbors++
					}
				}
			}
			if neighbors == 0 {
				out[idx] = false
			}
		}
	}
	return out
}

func morphDilateRect(fg []bool, w, h, rx, ry int) []bool {
	horiz := make([]bool, w*h)
	for y := 0; y < h; y++ {
		row := y * w
		run := 0
		for x := 0; x < w; x++ {
			if fg[row+x] {
				run = rx + 1
			}
			if run > 0 {
				horiz[row+x] = true
				run--
			}
		}
		run = 0
		for x := w - 1; x >= 0; x-- {
			if fg[row+x] {
				run = rx + 1
			}
			if run > 0 {
				horiz[row+x] = true
				run--
			}
		}
	}

	out := make([]bool, w*h)
	for x := 0; x < w; x++ {
		run := 0
		for y := 0; y < h; y++ {
			if horiz[y*w+x] {
				run = ry + 1
			}
			if run > 0 {
				out[y*w+x] = true
				run--
			}
		}
		run = 0
		for y := h - 1; y >= 0; y-- {
			if horiz[y*w+x] {
				run = ry + 1
			}
			if run > 0 {
				out[y*w+x] = true
				run--
			}
		}
	}
	return out
}

func connectedComponentBoxes(fg []bool, w, h int) []image.Rectangle {
	visited := make([]bool, w*h)
	queue := make([]int, 0, 256)
	var boxes []image.Rectangle

	for i := 0; i < w*h; i++ {
		if !fg[i] || visited[i] {
			continue
		}
		visited[i] = true
		queue = queue[:0]
		queue = append(queue, i)

		minX, minY := i%w, i/w
		maxX, maxY := minX, minY

		for head := 0; head < len(queue); head++ {
			curr := queue[head]
			cx, cy := curr%w, curr/w
			if cx < minX {
				minX = cx
			}
			if cx > maxX {
				maxX = cx
			}
			if cy < minY {
				minY = cy
			}
			if cy > maxY {
				maxY = cy
			}

			for dy := -1; dy <= 1; dy++ {
				ny := cy + dy
				if ny < 0 || ny >= h {
					continue
				}
				for dx := -1; dx <= 1; dx++ {
					nx := cx + dx
					if nx < 0 || nx >= w {
						continue
					}
					nIdx := ny*w + nx
					if fg[nIdx] && !visited[nIdx] {
						visited[nIdx] = true
						queue = append(queue, nIdx)
					}
				}
			}
		}
		boxes = append(boxes, image.Rect(minX, minY, maxX+1, maxY+1))
	}
	return boxes
}

func mergeOverlappingBoxes(boxes []image.Rectangle) []image.Rectangle {
	if len(boxes) <= 1 {
		return boxes
	}
	out := append([]image.Rectangle(nil), boxes...)
	changed := true
	for changed {
		changed = false
		next := make([]image.Rectangle, 0, len(out))
		used := make([]bool, len(out))
		for i := 0; i < len(out); i++ {
			if used[i] {
				continue
			}
			acc := out[i]
			for j := i + 1; j < len(out); j++ {
				if used[j] {
					continue
				}
				if !acc.Intersect(out[j]).Empty() {
					acc = acc.Union(out[j])
					used[j] = true
					changed = true
				}
			}
			next = append(next, acc)
		}
		out = next
	}
	return SortAndFilterBoxes(out, image.Rect(0, 0, 1<<20, 1<<20), 0, -1)
}

func maskToGrayImage(fg []bool, w, h int, darkInkOnWhite bool) *image.Gray {
	out := image.NewGray(image.Rect(0, 0, w, h))
	for i, isInk := range fg {
		val := uint8(255)
		if isInk {
			if darkInkOnWhite {
				val = 0
			} else {
				val = 255
			}
		} else if !darkInkOnWhite {
			val = 0
		}
		out.Pix[i] = val
	}
	return out
}

func maskToPaddedGrayImage(fg []bool, w, h, pad int) *image.Gray {
	pw, ph := w+2*pad, h+2*pad
	out := image.NewGray(image.Rect(0, 0, pw, ph))
	for i := range out.Pix {
		out.Pix[i] = 255
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if fg[y*w+x] {
				out.Pix[(y+pad)*pw+(x+pad)] = 0
			}
		}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
