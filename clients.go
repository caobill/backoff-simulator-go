package main

// WriteServer is what a WriteOnlyClient needs from a server.
type WriteServer interface {
	// HandleWrite processes a write from clientID. If the write is rejected,
	// the server calls onReject once the rejection has crossed the network.
	HandleWrite(clientID int, onReject func())
}

// ReadWriteServer is what a ReadWriteClient needs from a server.
type ReadWriteServer interface {
	// HandleRead reports the current version to the client via onVersion.
	HandleRead(clientID int, onVersion func(version int))
	// HandleWrite processes a write that the client based on version.
	// If the write is aborted, the server calls onAbort.
	HandleWrite(clientID, version int, onAbort func())
}

// WriteOnlyClient sends write requests to a server over the network.
// If a request succeeds, it stops. If it is rejected, it backs off and retries.
type WriteOnlyClient struct {
	sim      *Simulation
	id       int
	network  Network
	server   WriteServer
	strategy Strategy
	attempts int
}

func (c *WriteOnlyClient) Start() {
	c.sim.Record(ClientRequestsWrite, c.id, "")
	c.sim.Schedule(c.network.delay(c.sim.rng), func() { c.server.HandleWrite(c.id, c.backOff) })
}

func (c *WriteOnlyClient) backOff() {
	c.sim.Record(ClientBacksOff, c.id, "")
	delay := c.strategy.Delay(c.attempts, c.sim.rng)
	c.attempts++
	c.sim.Schedule(delay, c.Start) // client-internal, so no network delay
}

// ReadWriteClient reads the version, then requests a write passing that version.
// If the write succeeds, it stops. If it is aborted, it backs off and retries.
type ReadWriteClient struct {
	sim      *Simulation
	id       int
	network  Network
	server   ReadWriteServer
	strategy Strategy
	attempts int
}

func (c *ReadWriteClient) Start() {
	c.sim.Record(ClientRequestsVersion, c.id, "")
	c.sim.Schedule(c.network.delay(c.sim.rng), func() { c.server.HandleRead(c.id, c.requestWrite) })
}

func (c *ReadWriteClient) requestWrite(version int) {
	c.sim.Record(ClientRequestsWrite, c.id, "")
	c.sim.Schedule(c.network.delay(c.sim.rng), func() { c.server.HandleWrite(c.id, version, c.backOff) })
}

func (c *ReadWriteClient) backOff() {
	c.sim.Record(ClientBacksOff, c.id, "")
	delay := c.strategy.Delay(c.attempts, c.sim.rng)
	c.attempts++
	c.sim.Schedule(delay, c.Start) // client-internal, so no network delay
}
