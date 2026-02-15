package toggle

import "sync/atomic"

type Toggle struct {
	*atomic.Bool
}

func (d *Toggle) Set(enabled bool) {
	d.Store(enabled)
}

func (d *Toggle) Toggle() bool {
	en := !d.Load()
	d.Store(en)
	return en
}

func NewToggle(initial bool) *Toggle {
	ab := &atomic.Bool{}
	ab.Store(initial)

	return &Toggle{Bool: ab}
}
