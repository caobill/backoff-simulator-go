package main

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Spec describes one [[simulation]] block: which control and strategies to
// simulate, and over how many clients and repetitions.
type Spec struct {
	Title          string  `toml:"title"`
	MaxClients     int     `toml:"max_clients"`
	Repeat         int     `toml:"repeat"`
	NetworkMu      float64 `toml:"network_mu"`
	NetworkSigma   float64 `toml:"network_sigma"`
	WorkToDuration float64 `toml:"work_to_duration"`
	Control        string  `toml:"control"`

	// Control parameters. Which ones are required depends on Control.
	Limit      int     `toml:"limit"`       // ThrottlingServer
	Window     float64 `toml:"window"`      // ThrottlingServer
	Capacity   int     `toml:"capacity"`    // TokenBucketServer
	RefillRate float64 `toml:"refill_rate"` // TokenBucketServer
	WriteMu    float64 `toml:"write_mu"`    // Locking, WriteOnlyOCC, ReadWriteOCC
	WriteSigma float64 `toml:"write_sigma"` // Locking, WriteOnlyOCC, ReadWriteOCC

	Strategies []StrategySpec `toml:"strategies"`
}

// StrategySpec describes one backoff strategy. Which fields are required
// depends on Type.
type StrategySpec struct {
	Type     string  `toml:"type"`
	Constant float64 `toml:"constant"` // Constant
	Base     float64 `toml:"base"`     // Expo, FullJitteredExpo, EqualJitteredExpo
	Cap      float64 `toml:"cap"`
}

func (s StrategySpec) strategy() Strategy {
	switch s.Type {
	case "Constant":
		return Constant{s.Constant}
	case "Expo":
		return Expo{s.Base, s.Cap}
	case "FullJitteredExpo":
		return FullJitter{Expo{s.Base, s.Cap}}
	case "EqualJitteredExpo":
		return EqualJitter{Expo{s.Base, s.Cap}}
	}
	panic("unknown strategy " + s.Type) // loadConfig has already validated Type
}

// requiredKeys lists the control- or strategy-specific keys each type needs.
var requiredKeys = map[string][]string{
	"ThrottlingServer":   {"limit", "window"},
	"TokenBucketServer":  {"capacity", "refill_rate"},
	"LockingServer":      {"write_mu", "write_sigma"},
	"WriteOnlyOCCServer": {"write_mu", "write_sigma"},
	"ReadWriteOCCServer": {"write_mu", "write_sigma"},
	"Constant":           {"constant"},
	"Expo":               {"base", "cap"},
	"FullJitteredExpo":   {"base", "cap"},
	"EqualJitteredExpo":  {"base", "cap"},
}

// loadConfig reads and validates every [[simulation]] block in a TOML file.
func loadConfig(path string) ([]Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file struct {
		Simulation []Spec `toml:"simulation"`
	}
	md, err := toml.Decode(string(data), &file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("%s: unknown key %s", path, undecoded[0])
	}
	if len(file.Simulation) == 0 {
		return nil, fmt.Errorf("%s: no [[simulation]] blocks", path)
	}

	// Zero is a legitimate value for several keys (e.g. write_mu = 0.0), so
	// decode again into plain maps to check which keys are actually present.
	var raw struct {
		Simulation []map[string]any `toml:"simulation"`
	}
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i, spec := range file.Simulation {
		if err := validate(spec, raw.Simulation[i]); err != nil {
			return nil, fmt.Errorf("%s: simulation %d: %w", path, i+1, err)
		}
	}
	return file.Simulation, nil
}

func validate(spec Spec, raw map[string]any) error {
	for _, key := range []string{"title", "max_clients", "repeat", "network_mu", "network_sigma", "work_to_duration", "control", "strategies"} {
		if _, ok := raw[key]; !ok {
			return fmt.Errorf("missing %s", key)
		}
	}
	if spec.MaxClients < 1 {
		return fmt.Errorf("max_clients must be at least 1")
	}
	if spec.Repeat < 1 {
		return fmt.Errorf("repeat must be at least 1")
	}
	if len(spec.Strategies) == 0 {
		return fmt.Errorf("strategies must not be empty")
	}

	keys, ok := requiredKeys[spec.Control]
	if !ok {
		return fmt.Errorf("unknown control %q", spec.Control)
	}
	for _, key := range keys {
		if _, ok := raw[key]; !ok {
			return fmt.Errorf("%s requires %s", spec.Control, key)
		}
	}

	rawStrategies, _ := raw["strategies"].([]any)
	for i, s := range spec.Strategies {
		keys, ok := requiredKeys[s.Type]
		if !ok {
			return fmt.Errorf("unknown strategy %q", s.Type)
		}
		rawStrategy, _ := rawStrategies[i].(map[string]any)
		for _, key := range keys {
			if _, ok := rawStrategy[key]; !ok {
				return fmt.Errorf("%s requires %s", s.Type, key)
			}
		}
	}
	return nil
}
