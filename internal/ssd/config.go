// internal/ssd/config.go
package ssd

import (
	"math/rand"
	"time"
)

type Config struct {
	LatencyMs int     // fixed delay on every operation
	JitterMs  int     // random extra 0..JitterMs added on top
	FailRate  float64 // 0.0–1.0, probability of returning an error
}

func (c Config) apply() error {
	if c.FailRate > 0 && rand.Float64() < c.FailRate {
		return ErrInjected
	}
	d := time.Duration(c.LatencyMs) * time.Millisecond
	if c.JitterMs > 0 {
		d += time.Duration(rand.Intn(c.JitterMs)) * time.Millisecond
	}
	if d > 0 {
		time.Sleep(d)
	}
	return nil
}
