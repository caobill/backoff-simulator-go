package main

import (
	"fmt"
	"math"
	"math/rand/v2"
)

// Servers never tell clients the good news: a client assumes its request was
// accepted when it doesn't hear back. Server-internal follow-ups (freeing a
// lock, decrementing a window count) are scheduled without network delay.

// ThrottlingServer accepts up to limit requests in a sliding window and
// rejects the rest immediately.
type ThrottlingServer struct {
	sim     *Simulation
	network Network
	limit   int
	window  float64
	count   int // requests accepted within the current window
}

func (s *ThrottlingServer) HandleWrite(clientID int, onReject func()) {
	if s.count < 0 || s.count > s.limit {
		panic(fmt.Sprintf("throttling count %d outside [0, %d]", s.count, s.limit))
	}
	if s.count == s.limit {
		s.sim.Record(ServerRejects, clientID, "")
		s.sim.Schedule(s.network.delay(s.sim.rng), onReject)
		return
	}
	s.count++
	s.sim.Record(ServerAccepts, clientID, fmt.Sprintf("count=%d", s.count))
	s.sim.Schedule(s.window, func() {
		s.count--
		s.sim.Record(ServerDecrements, clientID, fmt.Sprintf("count=%d", s.count))
	})
}

// TokenBucketServer holds up to capacity tokens, refilled at refillRate per
// second. A write takes a token if one is available and is rejected otherwise.
type TokenBucketServer struct {
	sim        *Simulation
	network    Network
	capacity   int
	refillRate float64
	tokens     float64
	lastRefill float64 // simulated time the bucket was last topped up
}

func (s *TokenBucketServer) HandleWrite(clientID int, onReject func()) {
	// Refill lazily rather than scheduling refills forever, so the simulation
	// ends once every client has been accepted.
	s.tokens = min(float64(s.capacity), s.tokens+(s.sim.now-s.lastRefill)*s.refillRate)
	s.lastRefill = s.sim.now

	if s.tokens < 1 {
		s.sim.Record(ServerRejects, clientID, "")
		s.sim.Schedule(s.network.delay(s.sim.rng), onReject)
		return
	}
	s.tokens--
	s.sim.Record(ServerAccepts, clientID, fmt.Sprintf("tokens=%.2f", s.tokens))
	// Nothing further to do, but the history must end with a commit.
	s.sim.Record(ServerCommits, clientID, "")
}

// LockingServer accepts a write if it is available and becomes unavailable
// while writing; otherwise it rejects immediately.
type LockingServer struct {
	sim       *Simulation
	network   Network
	write     Normal // write duration
	available bool
}

func (s *LockingServer) HandleWrite(clientID int, onReject func()) {
	if !s.available {
		s.sim.Record(ServerRejects, clientID, "")
		s.sim.Schedule(s.network.delay(s.sim.rng), onReject)
		return
	}
	s.available = false
	s.sim.Record(ServerAccepts, clientID, "")
	s.sim.Schedule(s.write.abs(s.sim.rng), func() {
		s.available = true
		s.sim.Record(ServerCommits, clientID, "")
	})
}

// WriteOnlyOCCServer notes its version, tentatively writes, then commits if the
// version is unchanged and aborts otherwise. Writes are variable-duration;
// with fixed durations a write would commit exactly when none was in progress,
// which is just a locking server that does doomed work.
type WriteOnlyOCCServer struct {
	sim     *Simulation
	network Network
	write   Normal // write duration
	version int
}

func (s *WriteOnlyOCCServer) HandleWrite(clientID int, onReject func()) {
	s.sim.Record(ServerTentativelyWrite, clientID, "")
	version := s.version
	s.sim.Schedule(s.write.abs(s.sim.rng), func() { maybeCommit(s.sim, s.network, &s.version, clientID, version, onReject) })
}

// ReadWriteOCCServer is like WriteOnlyOCCServer, except the client reads the
// version first and passes it back with the write.
type ReadWriteOCCServer struct {
	sim     *Simulation
	network Network
	write   Normal // write duration
	version int
}

func (s *ReadWriteOCCServer) HandleRead(clientID int, onVersion func(version int)) {
	s.sim.Record(ServerReportsVersion, clientID, fmt.Sprintf("version=%d", s.version))
	version := s.version
	s.sim.Schedule(s.network.delay(s.sim.rng), func() { onVersion(version) })
}

func (s *ReadWriteOCCServer) HandleWrite(clientID, version int, onAbort func()) {
	s.sim.Record(ServerTentativelyWrite, clientID, "")
	s.sim.Schedule(s.write.abs(s.sim.rng), func() { maybeCommit(s.sim, s.network, &s.version, clientID, version, onAbort) })
}

// maybeCommit finishes a tentative write: it commits and bumps *current if no
// other write committed since the client's version was taken, else it aborts.
func maybeCommit(sim *Simulation, network Network, current *int, clientID, version int, onAbort func()) {
	if *current < version {
		panic(fmt.Sprintf("version decreased: %d < %d", *current, version))
	}
	if *current > version {
		sim.Record(ServerAborts, clientID, "")
		sim.Schedule(network.delay(sim.rng), onAbort)
		return
	}
	*current++
	sim.Record(ServerCommits, clientID, fmt.Sprintf("version=%d", *current))
}

// Normal is a normal distribution used for write durations, sampled as |N(mu, sigma)|.
type Normal struct {
	Mu, Sigma float64
}

func (n Normal) abs(rng *rand.Rand) float64 {
	return math.Abs(n.Mu + n.Sigma*rng.NormFloat64())
}
