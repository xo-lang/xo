package rt

// std/backoff on the Go backend: cenkalti/backoff v4's
// ExponentialBackOff and Retry (Copyright (c) 2014 Cenk Altı, MIT
// license), the same arithmetic as std/native/backoff.xo. Hand written:
// the Go backend does not yet instantiate a compiled generic std
// function from its call site (retry's T and E came out as Unit).

func backoffInterval(b Backoff_Backoff, attempt int64) Duration {
	ci := b.F_initial
	for k := int64(0); k < attempt && ci != b.F_max_interval; k++ {
		if float64(ci) >= float64(b.F_max_interval)/b.F_multiplier {
			ci = b.F_max_interval
		} else {
			ci = Duration(float64(ci) * b.F_multiplier)
		}
	}
	return ci
}

func backoffRandomized(factor, r float64, ci Duration) Duration {
	if factor == 0 {
		return ci
	}
	c := float64(ci)
	delta := factor * c
	lo, hi := c-delta, c+delta
	return Duration(lo + r*(hi-lo+1))
}

// M_delay is `b.delay(attempt, rand)`.
func (b Backoff_Backoff) M_delay(tk *Task, attempt int64, rand Rand) Duration {
	return backoffRandomized(b.F_randomization, rand.M_float(tk), backoffInterval(b, attempt))
}

// Backoff_retry is `retry(b, clock, rand, op, retryable)`.
func Backoff_retry[T, E any](tk *Task, b Backoff_Backoff, clock Clock, rand Rand, op func(*Task) Result[T, E], retryable func(*Task, E) Result[bool, Never]) Result[T, E] {
	start := clock.M_now(tk)
	tries := int64(0)
	for {
		tries++
		r := op(tk)
		if r.Ok {
			return r
		}
		if !retryable(tk, r.E).V || (b.F_max_tries > 0 && tries >= b.F_max_tries) {
			return r
		}
		d := b.M_delay(tk, tries-1, rand)
		if b.F_max_elapsed > 0 && clock.M_since(tk, start)+d > b.F_max_elapsed {
			return r
		}
		clock.M_sleep(tk, d)
	}
}

// Backoff_always is `always[E]()`.
func Backoff_always[E any](tk *Task) func(*Task, E) Result[bool, Never] {
	return func(*Task, E) Result[bool, Never] { return Ok[bool, Never](true) }
}
