package gocvf

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"

	"gocv.io/x/gocv"
	"gocv.io/x/gocv/contrib"
)

func Morph(img *gocv.Mat, kernalSize int, morph gocv.MorphType) (*gocv.Mat, error) {
	kernal := gocv.GetStructuringElement(gocv.MorphRect, image.Point{X: kernalSize, Y: kernalSize})

	openImage := gocv.NewMat()
	err := gocv.MorphologyEx(*img, &openImage, morph, kernal)
	if err != nil {
		openImage.Close()
		return nil, err
	}
	return &openImage, nil
}

func GaussianBlur(img *gocv.Mat, kernel image.Point, sigX, sigY float64, borderType gocv.BorderType) (*gocv.Mat, error) {
	gb := gocv.NewMat()
	err := gocv.GaussianBlur(*img, &gb, kernel, sigX, sigY, borderType)
	if err != nil {
		gb.Close()
		return nil, err
	}
	return &gb, nil
}

func AdaptiveThreshhold(img *gocv.Mat, max, c float32, blockSize int, thType gocv.ThresholdType, adThType gocv.AdaptiveThresholdType) (*gocv.Mat, error) {
	thresh := gocv.NewMat()
	err := gocv.AdaptiveThreshold(*img, &thresh, max, adThType, thType, blockSize, c)
	if err != nil {
		thresh.Close()
		return nil, err
	}
	return &thresh, nil
}

func NewStructuringElement(x, y int) *gocv.Mat {
	ste := gocv.GetStructuringElement(gocv.MorphRect, image.Point{X: x, Y: y})
	return &ste
}

func Erode(img *gocv.Mat, structuringElement *gocv.Mat) (*gocv.Mat, error) {
	er := gocv.NewMat()

	err := gocv.Erode(*img, &er, *structuringElement)
	if err != nil {
		er.Close()
		return nil, err
	}
	return &er, nil
}

func Dilate(img *gocv.Mat, structuringElement *gocv.Mat) (*gocv.Mat, error) {
	er := gocv.NewMat()

	err := gocv.Dilate(*img, &er, *structuringElement)
	if err != nil {
		er.Close()
		return nil, err
	}
	return &er, nil
}

func LineStructuringElement(direction bool, kernelLength int) *gocv.Mat {
	switch direction {
	case true:
		verticalKernel := gocv.GetStructuringElement(gocv.MorphRect, image.Point{X: 1, Y: kernelLength})
		return &verticalKernel
	default:
		horizontalKernel := gocv.GetStructuringElement(gocv.MorphRect, image.Point{X: kernelLength, Y: 1})
		return &horizontalKernel
	}
}

func ThresholdOtsu(img *gocv.Mat) *gocv.Mat {
	ne := gocv.NewMat()
	gocv.Threshold(*img, &ne, 0, 255, gocv.ThresholdBinary|gocv.ThresholdOtsu)
	return &ne
}

func CannyEdgeDetection(img *gocv.Mat, thMin, thMax float32) (*gocv.Mat, error) {
	edges := gocv.NewMat()
	err := gocv.Canny(*img, &edges, thMin, thMax)
	if err != nil {
		edges.Close()
		return nil, err
	}
	return &edges, nil
}

func FindContours(img *gocv.Mat, minArea, maxArea int, mode gocv.RetrievalMode, appMode gocv.ContourApproximationMode) []image.Rectangle {
	contours := gocv.FindContours(*img, mode, appMode)
	defer contours.Close()

	cb := ExtractCellBounds(contours)
	rects := SortAndFilterRects(cb, minArea, maxArea)

	return rects
}

func ExtractCellBounds(contours gocv.PointsVector) []image.Rectangle {
	var cellRects []image.Rectangle
	for i := 0; i < contours.Size(); i++ {
		contour := contours.At(i)
		rect := gocv.BoundingRect(contour)
		cellRects = append(cellRects, rect)
	}

	return cellRects
}

func SortAndFilterRects(rects []image.Rectangle, minArea, maxArea int) []image.Rectangle {
	// Filter out small bounding boxes. Adjust threshold as needed.
	minCellArea := minArea
	filtered := make([]image.Rectangle, 0)
	for _, rect := range rects {
		if maxArea != -1 {
			if rect.Dx()*rect.Dy() > minCellArea && rect.Dx()*rect.Dy() < maxArea {
				filtered = append(filtered, rect)
			}
			continue
		}

		if rect.Dx()*rect.Dy() > minCellArea {
			filtered = append(filtered, rect)
		}
	}

	// Sort the bounding boxes by y-coordinate, then by x-coordinate.
	sort.Slice(filtered, func(i, j int) bool {
		r1, r2 := filtered[i], filtered[j]
		if math.Abs(float64(r1.Min.Y-r2.Min.Y)) < 10 { // Y-proximity threshold
			return r1.Min.X < r2.Min.X
		}
		return r1.Min.Y < r2.Min.Y
	})

	return filtered
}

func GetHorizontalAndVerticalLines(binary *gocv.Mat, kernelLength int) (err error) {
	horizontalKernel := LineStructuringElement(false, kernelLength)
	defer horizontalKernel.Close()

	verticalKernel := LineStructuringElement(true, kernelLength)
	defer verticalKernel.Close()

	// Detect horizontal lines.
	horizontal := gocv.NewMat()
	defer horizontal.Close()

	err = gocv.Erode(*binary, &horizontal, *horizontalKernel)
	if err != nil {
		return err
	}

	err = gocv.Dilate(horizontal, &horizontal, *horizontalKernel)
	if err != nil {
		return err
	}

	// Detect vertical lines.
	vertical := gocv.NewMat()
	defer vertical.Close()

	err = gocv.Erode(*binary, &vertical, *verticalKernel)
	if err != nil {
		return err
	}

	err = gocv.Dilate(vertical, &vertical, *verticalKernel)
	if err != nil {
		return err
	}

	combined := gocv.NewMat()
	defer combined.Close()

	err = gocv.AddWeighted(horizontal, 0.5, vertical, 0.5, 0.0, &combined)
	if err != nil {
		return err
	}
	return nil
}

func ConvertToGrayScale(img *gocv.Mat) (*gocv.Mat, error) {
	gs := gocv.NewMat()
	err := gocv.CvtColor(*img, &gs, gocv.ColorBGRToGray)
	if err != nil {
		gs.Close()
		return nil, err
	}
	return &gs, nil
}

func InvertImage(img *gocv.Mat) (*gocv.Mat, error) {
	inv := gocv.NewMat()

	err := gocv.BitwiseNot(*img, &inv)
	if err != nil {
		inv.Close()
		return nil, err
	}
	return &inv, nil
}

func Resize(img *gocv.Mat, sz image.Point, fx, fy float64, flags gocv.InterpolationFlags) (*gocv.Mat, error) {
	rez := gocv.NewMat()
	err := gocv.Resize(*img, &rez, sz, fx, fy, flags)
	if err != nil {
		rez.Close()
		return nil, err
	}
	return &rez, nil
}

func HoughLinesP(img *gocv.Mat, rho, theta float32, thresh int) (*gocv.Mat, error) {
	lin := gocv.NewMat()
	err := gocv.HoughLinesP(*img, &lin, rho, theta, thresh)
	if err != nil {
		lin.Close()
		return nil, err
	}
	return &lin, nil
}

func HoughLinesPWithOptions(img *gocv.Mat, rho, theta float32, thresh int, minLength, maxLineGap float32) (*gocv.Mat, error) {
	lin := gocv.NewMat()
	err := gocv.HoughLinesPWithParams(*img, &lin, rho, theta, thresh, minLength, maxLineGap)
	if err != nil {
		lin.Close()
		return nil, err
	}
	return &lin, nil
}

func DrawLine(img *gocv.Mat, point1, point2 image.Point, col color.RGBA, thickness int) error {
	err := gocv.Line(img, point1, point2, col, thickness)
	if err != nil {
		return err
	}
	return nil
}

// DrawAngleLine draws a line representing the dominant angle on the image.
func DrawAngleLine(img *gocv.Mat, angleRad float64, col color.RGBA, thickness int) (err error) {
	height, width := img.Rows(), img.Cols()
	cx, cy := float64(width/2), float64(height/2)

	// Calculate a point on the line far from the center.
	lineLength := float64(math.Min(float64(width), float64(height))) / 2.0
	x1 := cx + lineLength*math.Cos(angleRad)
	y1 := cy + lineLength*math.Sin(angleRad)
	x2 := cx - lineLength*math.Cos(angleRad)
	y2 := cy - lineLength*math.Sin(angleRad)
	return DrawLine(img, image.Point{X: int(x1), Y: int(y1)}, image.Point{X: int(x2), Y: int(y2)}, col, thickness)
}

func DrawHoughLines(img *gocv.Mat, lines *gocv.Mat, col color.RGBA, thickness int) error {
	for i := 0; i < lines.Rows(); i++ {
		pt1 := image.Pt(int(lines.GetVeciAt(i, 0)[0]), int(lines.GetVeciAt(i, 0)[1]))
		pt2 := image.Pt(int(lines.GetVeciAt(i, 0)[2]), int(lines.GetVeciAt(i, 0)[3]))
		err := DrawLine(img, pt1, pt2, col, thickness)
		if err != nil {
			return err
		}
	}
	return nil
}

func GetRotationMatrix(img *gocv.Mat, angle float64, scale float64) *gocv.Mat {
	rows, cols := img.Rows(), img.Cols()
	center := image.Point{X: cols / 2, Y: rows / 2}
	rotationMatrix := gocv.GetRotationMatrix2D(center, angle, scale)
	return &rotationMatrix
}

func WarpAffine(img *gocv.Mat, rotationMatrix *gocv.Mat) (*gocv.Mat, error) {
	out := gocv.NewMat()
	err := gocv.WarpAffine(*img, &out, *rotationMatrix, image.Point{X: img.Cols(), Y: img.Rows()})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func LineAngles(lines *gocv.Mat) (dominantAngleDeg float64, dominantAngleRad float64) {
	// Variables for circular average calculation.
	var sumSin, sumCos float64

	// Iterate through detected lines to calculate angles and lengths.
	for i := 0; i < lines.Rows(); i++ {
		line := lines.RowRange(i, i+1)
		lineData := line.GetVeciAt(i, 0)

		x1, y1, x2, y2 := float64(lineData[0]), float64(lineData[1]), float64(lineData[2]), float64(lineData[3])
		line.Close()

		// Calculate angle in radians.
		angle := math.Atan2(y2-y1, x2-x1)

		// Ensure angle is in the range [0, π) for a line.
		if angle < 0 {
			angle += math.Pi
		}

		// Calculate length of the line.
		length := math.Sqrt(math.Pow(x2-x1, 2) + math.Pow(y2-y1, 2))

		// Sum sines and cosines weighted by length for circular average.
		sumSin += length * math.Sin(2*angle) // Multiply angle by 2 for line orientation.
		sumCos += length * math.Cos(2*angle) // A line at angle 'a' is the same as at 'a+180' degrees.
	}

	// Calculate the dominant angle.
	if sumCos == 0 && sumSin == 0 {
		dominantAngleRad = 0
	} else {
		dominantAngleRad = 0.5 * math.Atan2(sumSin, sumCos) // Divide by 2 to get back to line orientation.
		if dominantAngleRad < 0 {
			dominantAngleRad += math.Pi
		}
	}

	// Convert dominant angle to degrees for display.
	dominantAngleDeg = dominantAngleRad * 180 / math.Pi
	return dominantAngleDeg, dominantAngleRad
}

func Deskew(img *gocv.Mat, dominantAngleRad float64) (*gocv.Mat, error) {
	// The angle for deskewing is the negative of the dominant angle.
	// We subtract 90 degrees (pi/2) because a perfectly horizontal line is at 90 degrees (pi/2) in polar coordinates,
	// but the `GetRotationMatrix2D` function uses a standard cartesian system where 0 degrees is horizontal.
	// It's also important to convert back to degrees for the rotation function.
	deskewAngleDeg := dominantAngleRad*180/math.Pi - 90
	if dominantAngleRad*180/math.Pi > 90 {
		deskewAngleDeg = dominantAngleRad*180/math.Pi - 180
	}

	fmt.Printf("Detected dominant angle (radians): %.2f\n", dominantAngleRad)
	fmt.Printf("Deskewing angle (degrees): %.2f\n", deskewAngleDeg)

	rotationMatrix := GetRotationMatrix(img, deskewAngleDeg, 1.0)
	defer rotationMatrix.Close()

	deskewedImg, err := WarpAffine(img, rotationMatrix)
	if err != nil {
		return nil, err
	}
	return deskewedImg, nil
}

func ThresholdBinary(img *gocv.Mat, min, max float32) *gocv.Mat {
	thresh := gocv.NewMat()
	gocv.Threshold(*img, &thresh, min, max, gocv.ThresholdBinary)
	return &thresh
}

func Thinning(img *gocv.Mat, thinType contrib.ThinningType) (*gocv.Mat, error) {
	thin := gocv.NewMat()
	err := contrib.Thinning(*img, &thin, thinType)
	if err != nil {
		thin.Close()
		return nil, err
	}
	return &thin, nil
}

func FindMedianAngle(lines *gocv.Mat) float64 {
	var angles []float64
	// Iterate through the lines Mat
	for i := 0; i < lines.Rows(); i++ {
		line := lines.GetVeciAt(i, 0)
		x1, y1, x2, y2 := line[0], line[1], line[2], line[3]

		// Calculate angle in radians, then convert to degrees
		angleRad := math.Atan2(float64(y2-y1), float64(x2-x1))
		angleDeg := angleRad * 180 / math.Pi

		// Only consider near-horizontal lines
		if math.Abs(angleDeg) < 45 {
			angles = append(angles, angleDeg)
		}
	}

	if len(angles) == 0 {
		return 0
	}

	// Calculate the median angle to reduce outlier influence
	sort.Float64s(angles)

	return angles[len(angles)/2]
}

func DeskewByCannyHoughLines(input *gocv.Mat, rotate *gocv.Mat) (*gocv.Mat, error) {
	cny, err := CannyEdgeDetection(input, 50, 150)
	if err != nil {
		return nil, err
	}
	defer cny.Close()

	lin, err := HoughLinesP(cny, 1, math.Pi/180, 10)
	if err != nil {
		return nil, err
	}
	defer lin.Close()

	medianAngle := FindMedianAngle(lin)

	// 6. Rotate the original image
	rows, cols := input.Rows(), input.Cols()
	center := image.Point{X: cols / 2, Y: rows / 2}

	// Get the rotation matrix
	M := gocv.GetRotationMatrix2D(center, medianAngle, 1.0)
	defer M.Close()

	rotated := gocv.NewMat()
	// Apply the rotation using WarpAffine
	err = gocv.WarpAffine(*rotate, &rotated, M, image.Point{X: cols, Y: rows})
	if err != nil {
		return nil, err
	}
	return &rotated, nil
}

func AddWeighted(src1, src2 *gocv.Mat, alpha, beta, gamma float64) (*gocv.Mat, error) {
	dst := gocv.NewMat()

	err := gocv.AddWeighted(*src1, alpha, *src2, beta, gamma, &dst)
	if err != nil {
		dst.Close()
		return nil, err
	}

	return &dst, nil
}
