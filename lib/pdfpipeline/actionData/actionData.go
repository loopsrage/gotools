package actionData

import (
	"image"
	"sync"

	"gocv.io/x/gocv"
)

type ActionData struct {
	RequestId string
	Page      int
	Name      string
	PDFbytes  []byte
	FileBytes *NativeBufferLocker
	Output    string
	PageText  []byte
	TableData []byte

	Rect       image.Rectangle
	RectResult *sync.Map
	Image      *ImgLocker
}

type ImgLocker struct {
	*gocv.Mat
	*sync.RWMutex
}

func (i *ImgLocker) Region(rect image.Rectangle) gocv.Mat {
	i.RLock()
	defer i.RUnlock()
	return i.Mat.Region(rect)
}

type NativeBufferLocker struct {
	*gocv.NativeByteBuffer
	*sync.RWMutex
}

func (l *NativeBufferLocker) GetBytes() []byte {
	l.RLock()
	defer l.RUnlock()
	return l.NativeByteBuffer.GetBytes()
}

func (l *NativeBufferLocker) Len() int {
	l.RLock()
	defer l.RUnlock()
	return l.NativeByteBuffer.Len()
}

func NewNativeBufferLocker(locks *gocv.NativeByteBuffer) *NativeBufferLocker {
	return &NativeBufferLocker{
		NativeByteBuffer: locks,
		RWMutex:          &sync.RWMutex{},
	}
}

func NewImageLocker(img *gocv.Mat) *ImgLocker {
	return &ImgLocker{
		Mat:     img,
		RWMutex: &sync.RWMutex{},
	}
}
