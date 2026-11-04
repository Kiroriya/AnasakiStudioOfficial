package api

import "time"

// slow stretches a test time limit under the race detector, which runs CPU
// heavy work (bcrypt, PDF fonts, WASM) many times slower than a normal build.
func slow(d time.Duration) time.Duration {
	if raceEnabled {
		return d * 15
	}
	return d
}
