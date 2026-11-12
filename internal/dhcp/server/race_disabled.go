//go:build !race

package server

// raceDetectorEnabled reports whether this build runs under the race
// detector. The race instrumentation multiplies scheduling cost several-fold,
// which no longer fits inside the non-race slack of the DDNS latency test.
const raceDetectorEnabled = false
