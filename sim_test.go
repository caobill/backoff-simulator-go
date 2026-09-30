package main

import (
	"bytes"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata/")

// specs covers every server type with a strategy from every family.
var specs = []Spec{
	{Title: "throttling", Control: "ThrottlingServer", Limit: 2, Window: 5, Strategies: []StrategySpec{{Type: "Constant", Constant: 1}}},
	{Title: "token_bucket", Control: "TokenBucketServer", Capacity: 2, RefillRate: 0.5, Strategies: []StrategySpec{{Type: "Expo", Base: 1, Cap: 32}}},
	{Title: "locking", Control: "LockingServer", WriteMu: 1, WriteSigma: 0.5, Strategies: []StrategySpec{{Type: "FullJitteredExpo", Base: 1, Cap: 32}}},
	{Title: "write_only_occ", Control: "WriteOnlyOCCServer", WriteMu: 1, WriteSigma: 0.5, Strategies: []StrategySpec{{Type: "EqualJitteredExpo", Base: 1, Cap: 32}}},
	{Title: "read_write_occ", Control: "ReadWriteOCCServer", WriteMu: 1, WriteSigma: 0.5, Strategies: []StrategySpec{{Type: "FullJitteredExpo", Base: 1, Cap: 32}}},
}

func runSpec(t *testing.T, spec Spec, n int) History {
	t.Helper()
	spec.NetworkMu, spec.NetworkSigma = 5, 1
	return build(spec, n, spec.Strategies[0].strategy(), rand.New(rand.NewPCG(1, 0))).Run()
}

// TestInvariants checks properties that hold regardless of the random seed.
func TestInvariants(t *testing.T) {
	for _, spec := range specs {
		t.Run(spec.Title, func(t *testing.T) {
			const n = 5
			h := runSpec(t, spec, n)

			if h.Duration() <= 0 {
				t.Errorf("duration = %v, want > 0", h.Duration())
			}
			if h.Work() < n {
				t.Errorf("work = %d, want at least %d", h.Work(), n)
			}
			finished := map[int]int{}
			for i, e := range h {
				if i > 0 && e.Time < h[i-1].Time {
					t.Errorf("event %d at %v is before event %d at %v", i, e.Time, i-1, h[i-1].Time)
				}
				if e.Type == ServerCommits || e.Type == ServerDecrements {
					finished[e.ClientID]++
				}
			}
			for id := range n {
				if finished[id] != 1 {
					t.Errorf("client %d finished %d times, want 1", id, finished[id])
				}
			}
		})
	}
}

// TestGolden compares seeded histories with testdata/<title>.txt.
// Run `go test -update` to regenerate them after an intentional change.
func TestGolden(t *testing.T) {
	for _, spec := range specs {
		t.Run(spec.Title, func(t *testing.T) {
			var got bytes.Buffer
			for _, e := range runSpec(t, spec, 3) {
				fmt.Fprintf(&got, "%8.3f  %d  %-25s %s\n", e.Time, e.ClientID, e.Type, e.Detail)
			}
			path := filepath.Join("testdata", spec.Title+".txt")
			if *update {
				if err := os.WriteFile(path, got.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Errorf("history differs from %s (run with -update to accept):\n%s", path, got.String())
			}
		})
	}
}

func TestClientNums(t *testing.T) {
	if got := clientNums(3); fmt.Sprint(got) != "[1 2 3]" {
		t.Errorf("clientNums(3) = %v", got)
	}
	got := clientNums(100)
	if len(got) != 20 || got[0] != 1 || got[19] != 100 {
		t.Errorf("clientNums(100) = %v", got)
	}
}
