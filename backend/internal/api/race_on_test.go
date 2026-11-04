//go:build race

package api

// raceEnabled: built with -race, which makes code 5-20× slower.
const raceEnabled = true
