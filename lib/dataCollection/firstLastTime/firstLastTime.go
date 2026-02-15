package firstLastTime

import (
	"sync/atomic"
	"time"
)

type FirstLast struct {
	ft int64
	lt int64
}

func (l *FirstLast) FirstTime() time.Time {
	return time.Unix(atomic.LoadInt64(&l.ft), 0)
}

func (l *FirstLast) LastTime() time.Time {
	return time.Unix(atomic.LoadInt64(&l.lt), 0)
}

func (l *FirstLast) SetFirstTime(tx time.Time) {
	if atomic.LoadInt64(&l.ft) == 0 || tx.Before(l.FirstTime()) {
		atomic.StoreInt64(&l.ft, tx.Unix())
	}
}

func (l *FirstLast) SetLastTime(tx time.Time) {
	if atomic.LoadInt64(&l.lt) == 0 || tx.After(l.LastTime()) {
		atomic.StoreInt64(&l.lt, tx.Unix())
	}
}

func (l *FirstLast) SetTimes(tx time.Time) {
	l.SetFirstTime(tx)
	l.SetLastTime(tx)
}
