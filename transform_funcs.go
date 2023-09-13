package main

import "math"

// TransCenteredRolloff applies a scaled tanh to ensure output values are between -1 and 1.
// The nominalRange should be a value that is on the higher side of what the input would normally be.
func TransCenteredRolloff(x, nominalRange float64) float64 {
	return math.Tanh(x / nominalRange)
}
