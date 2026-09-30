package main

import (
	"container/heap"
	"math/rand/v2"
)

// EventType names something of interest that happened during a simulation.
type EventType string

const (
	ClientRequestsWrite    EventType = "client_requests_write" // counting requests is counting these
	ClientRequestsVersion  EventType = "client_requests_version"
	ClientBacksOff         EventType = "client_backs_off"
	ServerAccepts          EventType = "server_accepts"
	ServerRejects          EventType = "server_rejects"
	ServerReportsVersion   EventType = "server_reports_version"
	ServerTentativelyWrite EventType = "server_tentatively_writes"
	ServerAborts           EventType = "server_aborts"
	ServerDecrements       EventType = "server_decrements" // last event for the throttling server
	ServerCommits          EventType = "server_commits"    // last event for the other servers
)

// Event is one entry in a simulation's history.
type Event struct {
	Time     float64
	Type     EventType
	ClientID int
	Detail   string // human-readable, free-form
}

// History is the timed sequence of everything that happened in a simulation.
type History []Event

// Work is the total number of client write requests.
func (h History) Work() int {
	n := 0
	for _, e := range h {
		if e.Type == ClientRequestsWrite {
			n++
		}
	}
	return n
}

// Duration is the time at which all client requests were fully dealt with.
func (h History) Duration() float64 {
	last := h[len(h)-1]
	if last.Type != ServerCommits && last.Type != ServerDecrements {
		panic("history does not end with a commit or decrement: " + string(last.Type))
	}
	return last.Time
}

// Simulation is a discrete-event simulator: a virtual clock, a queue of
// pending actions, and a record of what happened. Servers and clients hold a
// *Simulation and use it to schedule their own actions and record events.
type Simulation struct {
	now     float64
	queue   eventQueue
	rng     *rand.Rand
	history History
}

func NewSimulation(rng *rand.Rand) *Simulation {
	return &Simulation{rng: rng}
}

// Schedule arranges for fn to run delay seconds from now.
func (s *Simulation) Schedule(delay float64, fn func()) {
	if delay < 0 {
		panic("negative delay")
	}
	heap.Push(&s.queue, &action{at: s.now + delay, seq: len(s.history) + s.queue.Len(), run: fn})
}

// Record appends an event at the current time.
func (s *Simulation) Record(t EventType, clientID int, detail string) {
	s.history = append(s.history, Event{Time: s.now, Type: t, ClientID: clientID, Detail: detail})
}

// Run processes actions in time order until none remain and returns the history.
func (s *Simulation) Run() History {
	for s.queue.Len() > 0 {
		a := heap.Pop(&s.queue).(*action)
		s.now = a.at
		a.run()
	}
	return s.history
}

// action is a function scheduled to run at a point in simulated time.
type action struct {
	at  float64
	seq int // breaks ties so that actions scheduled first run first
	run func()
}

// eventQueue is a min-heap of actions, used only through container/heap.
type eventQueue []*action

func (q eventQueue) Len() int { return len(q) }
func (q eventQueue) Less(i, j int) bool {
	if q[i].at != q[j].at {
		return q[i].at < q[j].at
	}
	return q[i].seq < q[j].seq
}
func (q eventQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *eventQueue) Push(x any)   { *q = append(*q, x.(*action)) }
func (q *eventQueue) Pop() any {
	old := *q
	n := len(old)
	a := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return a
}

// Network simulates network latency as max(0, N(mu, sigma)).
type Network struct {
	Mu, Sigma float64
}

func (n Network) delay(rng *rand.Rand) float64 {
	return max(0, n.Mu+n.Sigma*rng.NormFloat64())
}
