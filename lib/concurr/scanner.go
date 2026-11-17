package concurr

import (
	"sync"
	"sync/atomic"
)

type Scanner struct {
	index    int64
	position int64
	count    int64
	data     *sync.Map
}

func NewScanner() *Scanner {
	return &Scanner{data: &sync.Map{}}
}

func (s *Scanner) Count() int64 {
	return atomic.LoadInt64(&s.count)
}

func (s *Scanner) Next() bool {

	return atomic.LoadInt64(&s.index) > atomic.LoadInt64(&s.position)
}

func (s *Scanner) Data() (position int64, data interface{}) {
	defer atomic.AddInt64(&s.position, 1)
	pos := atomic.LoadInt64(&s.position)
	return pos, s.At(pos)
}

func (s *Scanner) Add(data interface{}) {
	ci := atomic.LoadInt64(&s.index)
	s.data.Store(ci, data)
	atomic.AddInt64(&s.index, 1)
	atomic.AddInt64(&s.count, 1)
}

func (s *Scanner) At(position int64) interface{} {
	x, _ := s.data.Load(position)
	return x
}

func (s *Scanner) Reset() {
	atomic.StoreInt64(&s.index, 0)
	atomic.StoreInt64(&s.position, 0)
	atomic.StoreInt64(&s.count, 0)
	s.data = &sync.Map{}
}
