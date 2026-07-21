package config

import (
	"sync"
	"testing"
)

func TestSnapshotConcurrentLoadAndStore(t *testing.T) {
	snapshot := NewSnapshot(&Config{HTTP: HTTPConfig{RedirectToHTTPS: true}})

	const iterations = 1_000
	var readers sync.WaitGroup
	for range 8 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for range iterations {
				if snapshot.Load() == nil {
					t.Error("loaded a nil configuration")
				}
			}
		}()
	}

	for i := range iterations {
		snapshot.Store(&Config{HTTP: HTTPConfig{RedirectToHTTPS: i%2 == 0}})
	}
	readers.Wait()
}
