package main

import (
	"math"
	"math/rand/v2"
)

// Strategy decides how long a client waits before its n-th retry (n starts at 0).
type Strategy interface {
	Delay(attempt int, rng *rand.Rand) float64
}

// Constant backs off for the same time every attempt: c, c, c, ...
type Constant struct {
	Constant float64
}

func (c Constant) Delay(int, *rand.Rand) float64 { return c.Constant }

// Expo backs off exponentially, capped: min(cap, base * 2^n).
type Expo struct {
	Base, Cap float64
}

func (e Expo) Delay(attempt int, _ *rand.Rand) float64 {
	return min(e.Cap, e.Base*math.Pow(2, float64(attempt)))
}

// FullJitter picks uniformly from [0, expo).
type FullJitter struct {
	Expo
}

func (f FullJitter) Delay(attempt int, rng *rand.Rand) float64 {
	return rng.Float64() * f.Expo.Delay(attempt, rng)
}

// EqualJitter picks uniformly from [expo/2, expo).
type EqualJitter struct {
	Expo
}

func (e EqualJitter) Delay(attempt int, rng *rand.Rand) float64 {
	half := e.Expo.Delay(attempt, rng) / 2
	return half + rng.Float64()*half
}
