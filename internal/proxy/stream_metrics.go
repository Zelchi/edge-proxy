package proxy

import (
	"sort"
	"sync"
	"sync/atomic"
)

// StreamMetrics contains bounded, configuration-derived metrics for TCP and
// UDP listeners. It intentionally does not use client IPs as labels.
type StreamMetrics struct {
	mu        sync.RWMutex
	listeners map[string]*StreamListenerMetrics
}

type StreamListenerMetrics struct {
	protocol string
	name     string
	address  string

	active      atomic.Int64
	connections atomic.Uint64
	bytesIn     atomic.Uint64
	bytesOut    atomic.Uint64
	errors      atomic.Uint64
	rejected    atomic.Uint64

	mu        sync.RWMutex
	upstreams map[string]*StreamUpstreamMetrics
}

type StreamUpstreamMetrics struct {
	active      atomic.Int64
	connections atomic.Uint64
	bytesIn     atomic.Uint64
	bytesOut    atomic.Uint64
	errors      atomic.Uint64
}

type StreamConnectionMetrics struct {
	listener  *StreamListenerMetrics
	upstream  *StreamUpstreamMetrics
	closeOnce sync.Once
}

type StreamListenerSnapshot struct {
	Protocol    string                   `json:"protocol"`
	Name        string                   `json:"name"`
	Address     string                   `json:"address"`
	Active      int64                    `json:"active"`
	Connections uint64                   `json:"connections"`
	BytesIn     uint64                   `json:"bytes_in"`
	BytesOut    uint64                   `json:"bytes_out"`
	Errors      uint64                   `json:"errors"`
	Rejected    uint64                   `json:"rejected"`
	Upstreams   []StreamUpstreamSnapshot `json:"upstreams,omitempty"`
}

type StreamUpstreamSnapshot struct {
	Upstream    string `json:"upstream"`
	Active      int64  `json:"active"`
	Connections uint64 `json:"connections"`
	BytesIn     uint64 `json:"bytes_in"`
	BytesOut    uint64 `json:"bytes_out"`
	Errors      uint64 `json:"errors"`
}

func NewStreamMetrics() *StreamMetrics {
	return &StreamMetrics{listeners: make(map[string]*StreamListenerMetrics)}
}

func (m *StreamMetrics) Register(protocol, name, address string, upstreams []string) *StreamListenerMetrics {
	key := protocol + "\x00" + name
	m.mu.Lock()
	defer m.mu.Unlock()
	if listener, exists := m.listeners[key]; exists {
		return listener
	}
	listener := &StreamListenerMetrics{
		protocol:  protocol,
		name:      name,
		address:   address,
		upstreams: make(map[string]*StreamUpstreamMetrics, len(upstreams)),
	}
	for _, upstream := range upstreams {
		if upstream != "" {
			listener.upstreams[upstream] = &StreamUpstreamMetrics{}
		}
	}
	m.listeners[key] = listener
	return listener
}

func (m *StreamMetrics) Snapshot() []StreamListenerSnapshot {
	m.mu.RLock()
	listeners := make([]*StreamListenerMetrics, 0, len(m.listeners))
	for _, listener := range m.listeners {
		listeners = append(listeners, listener)
	}
	m.mu.RUnlock()
	sort.Slice(listeners, func(i, j int) bool {
		if listeners[i].protocol == listeners[j].protocol {
			return listeners[i].name < listeners[j].name
		}
		return listeners[i].protocol < listeners[j].protocol
	})

	snapshots := make([]StreamListenerSnapshot, 0, len(listeners))
	for _, listener := range listeners {
		snapshot := StreamListenerSnapshot{
			Protocol:    listener.protocol,
			Name:        listener.name,
			Address:     listener.address,
			Active:      listener.active.Load(),
			Connections: listener.connections.Load(),
			BytesIn:     listener.bytesIn.Load(),
			BytesOut:    listener.bytesOut.Load(),
			Errors:      listener.errors.Load(),
			Rejected:    listener.rejected.Load(),
		}
		listener.mu.RLock()
		upstreamNames := make([]string, 0, len(listener.upstreams))
		for upstream := range listener.upstreams {
			upstreamNames = append(upstreamNames, upstream)
		}
		sort.Strings(upstreamNames)
		for _, upstream := range upstreamNames {
			stats := listener.upstreams[upstream]
			snapshot.Upstreams = append(snapshot.Upstreams, StreamUpstreamSnapshot{
				Upstream:    upstream,
				Active:      stats.active.Load(),
				Connections: stats.connections.Load(),
				BytesIn:     stats.bytesIn.Load(),
				BytesOut:    stats.bytesOut.Load(),
				Errors:      stats.errors.Load(),
			})
		}
		listener.mu.RUnlock()
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func (m *StreamListenerMetrics) Begin(upstream string) *StreamConnectionMetrics {
	m.connections.Add(1)
	m.active.Add(1)
	stats := m.upstream(upstream)
	if stats != nil {
		stats.connections.Add(1)
		stats.active.Add(1)
	}
	return &StreamConnectionMetrics{listener: m, upstream: stats}
}

func (m *StreamListenerMetrics) Reject() {
	m.rejected.Add(1)
}

func (m *StreamListenerMetrics) Error() {
	m.errors.Add(1)
}

func (m *StreamListenerMetrics) upstream(name string) *StreamUpstreamMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.upstreams[name]
}

func (m *StreamConnectionMetrics) AddIn(bytes uint64) {
	m.listener.bytesIn.Add(bytes)
	if m.upstream != nil {
		m.upstream.bytesIn.Add(bytes)
	}
}

func (m *StreamConnectionMetrics) AddOut(bytes uint64) {
	m.listener.bytesOut.Add(bytes)
	if m.upstream != nil {
		m.upstream.bytesOut.Add(bytes)
	}
}

func (m *StreamConnectionMetrics) Error() {
	m.listener.errors.Add(1)
	if m.upstream != nil {
		m.upstream.errors.Add(1)
	}
}

func (m *StreamConnectionMetrics) Close() {
	m.closeOnce.Do(func() {
		m.listener.active.Add(-1)
		if m.upstream != nil {
			m.upstream.active.Add(-1)
		}
	})
}
