//go:build !race

package dataplane

// raceDetectorEnabled reports whether this test binary was built with the race
// detector. It exists so a wall-clock bound can say which binary it is talking
// about: the instrumented one is not the one the bound was measured on.
const raceDetectorEnabled = false
