package config

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

const configReloadDebounce = 100 * time.Millisecond

// WatchConfig watches the parent directory so updates made by replacing the
// config file (the usual atomic-save pattern) are detected too.
func WatchConfig(ctx context.Context, path string, onChange func()) error {
	return watchConfig(ctx, path, onChange, nil)
}

func watchConfig(ctx context.Context, path string, onChange func(), onReady func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create config watcher: %w", err)
	}
	defer watcher.Close()

	target := filepath.Clean(path)
	if err := watcher.Add(filepath.Dir(target)); err != nil {
		return fmt.Errorf("watch config directory: %w", err)
	}
	if onReady != nil {
		onReady()
	}

	var timer *time.Timer
	var timerC <-chan time.Time
	defer stopTimer(timer)

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if filepath.Clean(event.Name) != target || !isConfigUpdate(event.Op) {
				continue
			}
			stopTimer(timer)
			timer = time.NewTimer(configReloadDebounce)
			timerC = timer.C
		case <-timerC:
			timer = nil
			timerC = nil
			onChange()
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			return fmt.Errorf("watch config: %w", err)
		}
	}
}

func isConfigUpdate(operation fsnotify.Op) bool {
	return operation&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0
}

func stopTimer(timer *time.Timer) {
	if timer == nil || !timer.Stop() {
		return
	}
}
