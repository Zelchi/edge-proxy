package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchConfigDetectsReplacement(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.yml")
	if err := os.WriteFile(path, []byte("initial"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := make(chan struct{}, 1)
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- watchConfig(ctx, path, func() { changed <- struct{}{} }, func() { close(ready) })
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("watcher did not become ready")
	}

	temporaryPath := filepath.Join(directory, "config.yml.tmp")
	if err := os.WriteFile(temporaryPath, []byte("updated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		t.Fatal(err)
	}

	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not detect replacement")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WatchConfig() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("watcher did not stop after context cancellation")
	}
}
