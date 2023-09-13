package main

// This file just has some generic Quality-of-Life functions to make the main code more concise

import "cmp"

// clamp a number between two values
func clamp[T cmp.Ordered](x, xMin, xMax T) T {
	if x < xMin {
		return xMin
	}
	if x > xMax {
		return xMax
	}
	return x
}

// Apply a function to a slice of things
func apply[T, U any](f func(T) U, ts []T) []U {
	us := make([]U, len(ts))
	for i, t := range ts {
		us[i] = f(t)
	}
	return us
}

// Run a function a number of times to make a slice
func repeated[T any](f func() T, n int) []T {
	ts := make([]T, n)
	for i := range ts {
		ts[i] = f()
	}
	return ts
}

// Panics if a two value return function retuens an error. If not, returns the value
func must[T any](x T, err error) T {
	if err != nil {
		panic(err)
	}
	return x
}
