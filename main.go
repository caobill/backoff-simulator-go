package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"text/tabwriter"
)

func main() {
	configPath := flag.String("config", "simulations.toml", "path to config file")
	seed := flag.Uint64("seed", 1, "random seed")
	flag.Parse()

	specs, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, spec := range specs {
		results := simulate(spec, *seed)
		printMetrics(os.Stdout, spec, results)
		printHistories(os.Stdout, spec, results)
	}
}

// key identifies one group of repeated simulations.
type key struct {
	clients  int
	strategy string
}

// simulate runs every (strategy, number of clients, repetition) combination
// for a spec and returns the histories grouped by strategy and client count.
func simulate(spec Spec, seed uint64) map[key][]History {
	results := map[key][]History{}
	run := 0
	for _, s := range spec.Strategies {
		strategy := s.strategy()
		for _, n := range clientNums(spec.MaxClients) {
			for range spec.Repeat {
				// One rng per simulation, so each is reproducible from the seed.
				rng := rand.New(rand.NewPCG(seed, uint64(run)))
				run++
				history := build(spec, n, strategy, rng).Run()
				k := key{n, s.Type}
				results[k] = append(results[k], history)
			}
		}
	}
	return results
}

// build wires up a server and n clients on a fresh simulation, ready to run.
// It is the one place that knows which client goes with which server.
func build(spec Spec, n int, strategy Strategy, rng *rand.Rand) *Simulation {
	sim := NewSimulation(rng)
	network := Network{Mu: spec.NetworkMu, Sigma: spec.NetworkSigma}
	write := Normal{Mu: spec.WriteMu, Sigma: spec.WriteSigma}

	var server WriteServer
	switch spec.Control {
	case "ThrottlingServer":
		server = &ThrottlingServer{sim: sim, network: network, limit: spec.Limit, window: spec.Window}
	case "TokenBucketServer":
		server = &TokenBucketServer{sim: sim, network: network, capacity: spec.Capacity, refillRate: spec.RefillRate, tokens: float64(spec.Capacity)}
	case "LockingServer":
		server = &LockingServer{sim: sim, network: network, write: write, available: true}
	case "WriteOnlyOCCServer":
		server = &WriteOnlyOCCServer{sim: sim, network: network, write: write}
	case "ReadWriteOCCServer":
		server := &ReadWriteOCCServer{sim: sim, network: network, write: write}
		for id := range n {
			c := &ReadWriteClient{sim: sim, id: id, network: network, server: server, strategy: strategy}
			c.Start()
		}
		return sim
	default:
		panic("unknown control " + spec.Control) // loadConfig has already validated Control
	}
	for id := range n {
		c := &WriteOnlyClient{sim: sim, id: id, network: network, server: server, strategy: strategy}
		c.Start()
	}
	return sim
}

// clientNums returns up to 20 client counts spread evenly over 1..maxClients,
// always including both ends. Simulating every count would be slow and
// tell us little more.
func clientNums(maxClients int) []int {
	const maxValues = 20
	if maxClients <= maxValues {
		nums := make([]int, maxClients)
		for i := range nums {
			nums[i] = i + 1
		}
		return nums
	}
	step := float64(maxClients-1) / (maxValues - 1)
	nums := make([]int, maxValues)
	for i := range nums {
		nums[i] = int(math.Round(1 + float64(i)*step))
	}
	return nums
}

// printMetrics prints work, duration and cost averaged over repetitions,
// one row per (clients, strategy).
func printMetrics(w *os.File, spec Spec, results map[key][]History) {
	fmt.Fprintf(w, "\n%s (%s)\n\n", spec.Title, spec.Control)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "clients\tstrategy\twork\tduration\tcost\t")
	for _, n := range clientNums(spec.MaxClients) {
		for _, s := range spec.Strategies {
			histories := results[key{n, s.Type}]
			var work, duration float64
			for _, h := range histories {
				work += float64(h.Work())
				duration += h.Duration()
			}
			work /= float64(len(histories))
			duration /= float64(len(histories))
			cost := spec.WorkToDuration*work + duration
			fmt.Fprintf(tw, "%d\t%s\t%.1f\t%.2f\t%.2f\t\n", n, s.Type, work, duration, cost)
		}
	}
	tw.Flush()
}

// printHistories prints one representative history per strategy, from the
// smallest client count above 2 (or the largest available) so it's readable.
func printHistories(w *os.File, spec Spec, results map[key][]History) {
	nums := clientNums(spec.MaxClients)
	n := nums[len(nums)-1]
	if i := slices.IndexFunc(nums, func(n int) bool { return n > 2 }); i >= 0 {
		n = nums[i]
	}
	for _, s := range spec.Strategies {
		fmt.Fprintf(w, "\n%s + %s (%d clients)\n\n", spec.Title, s.Type, n)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "time\tclient\tevent\tdetail")
		for _, e := range results[key{n, s.Type}][0] {
			fmt.Fprintf(tw, "%.2f\t%d\t%s\t%s\n", e.Time, e.ClientID, e.Type, e.Detail)
		}
		tw.Flush()
	}
}
