package positions

import "time"

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }
func (RealClock) NewTicker(interval time.Duration) Ticker {
	return realTicker{time.NewTicker(interval)}
}

type realTicker struct{ *time.Ticker }

func (t realTicker) C() <-chan time.Time { return t.Ticker.C }
