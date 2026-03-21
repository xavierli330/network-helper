// internal/watcher/watcher.go
package watcher

import (
	"log/slog"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Config configures the file watcher.
type Config struct {
	Dirs         []string
	Debounce     time.Duration
	OnFileChange func(path string)
}

// Watcher monitors directories for file changes with debouncing.
type Watcher struct {
	config  Config
	fsw     *fsnotify.Watcher
	stop    chan struct{}
	stopped chan struct{}
	mu      sync.Mutex
	timers  map[string]*time.Timer
}

func New(cfg Config) (*Watcher, error) {
	if cfg.Debounce == 0 {
		cfg.Debounce = 500 * time.Millisecond
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	for _, dir := range cfg.Dirs {
		if err := fsw.Add(dir); err != nil {
			fsw.Close()
			return nil, err
		}
	}

	return &Watcher{
		config:  cfg,
		fsw:     fsw,
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
		timers:  make(map[string]*time.Timer),
	}, nil
}

func (w *Watcher) Start() {
	defer close(w.stopped)

	for {
		select {
		case <-w.stop:
			return
		case event, ok := <-w.fsw.Events:
			if !ok { return }
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 { continue }
			w.debounce(event.Name)
		case err, ok := <-w.fsw.Errors:
			if !ok { return }
			slog.Error("watcher error", "error", err)
		}
	}
}

func (w *Watcher) debounce(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if timer, exists := w.timers[path]; exists {
		timer.Stop()
	}

	w.timers[path] = time.AfterFunc(w.config.Debounce, func() {
		w.mu.Lock()
		delete(w.timers, path)
		w.mu.Unlock()

		if w.config.OnFileChange != nil {
			w.config.OnFileChange(path)
		}
	})
}

func (w *Watcher) Stop() {
	close(w.stop)
	w.fsw.Close()
	<-w.stopped

	// Cancel pending timers
	w.mu.Lock()
	for _, t := range w.timers { t.Stop() }
	w.timers = nil
	w.mu.Unlock()
}
