package config

import "sync/atomic"

// Snapshot safely publishes immutable configuration instances to concurrent
// request handlers.
type Snapshot struct {
	value atomic.Pointer[Config]
}

func NewSnapshot(initial *Config) *Snapshot {
	snapshot := &Snapshot{}
	snapshot.Store(initial)
	return snapshot
}

func (s *Snapshot) Load() *Config {
	return s.value.Load()
}

func (s *Snapshot) Store(cfg *Config) {
	if cfg == nil {
		panic("config snapshot cannot store nil")
	}
	s.value.Store(cfg)
}
