package main

import (
	"time"
)

type FakeClock struct {
	now time.Time
}

func (f *FakeClock) Now() time.Time { return f.now }
func (f *FakeClock) After(d time.Duration) <-chan time.Time {
	// In a real fake, this would trigger after d
	return make(<-chan time.Time)
}
