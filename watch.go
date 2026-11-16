package sailor

import (
	"path/filepath"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

// startWatcher watches the directories of VOLUME and DEV resources when Watch
// is enabled (CONS-VOL-3, CONS-DEV-6). A watcher that cannot start is logged
// and the consumer keeps serving the values it has (CONS-DEV-7 and CONS-ERR-2).
func (c *Consumer) startWatcher() {
	if !c.watch {
		return
	}
	dirs := map[string]bool{}
	for _, r := range c.resources {
		if (r.mode == opts.VOLUME || r.mode == opts.DEV) && r.dir != "" {
			dirs[r.dir] = true
		}
	}
	if len(dirs) == 0 {
		return
	}

	w, err := c.deps.newWatcher()
	if err != nil {
		c.emit(opts.LevelError, "watcher.failed", "path", "", "cause", err.Error())
		return
	}
	for dir := range dirs {
		if err := w.Add(dir); err != nil {
			c.emit(opts.LevelError, "watcher.failed", "path", dir, "cause", err.Error())
		}
	}

	c.mu.Lock()
	c.watcher = w
	c.mu.Unlock()
	c.spawn(func() { c.watchLoop(w) })
}

func (c *Consumer) watchLoop(w watcher) {
	pending := map[*resource]bool{}
	var timer <-chan time.Time
	for {
		select {
		case <-c.ctx.Done():
			return
		case ev, ok := <-w.Events():
			if !ok {
				return
			}
			c.markDirty(pending, ev.Path)
			timer = c.clock.After(watchDebounce)
		case err := <-w.Errors():
			c.emit(opts.LevelError, "watcher.failed", "path", "", "cause", err.Error())
		case <-timer:
			timer = nil
			for r := range pending {
				c.reread(r)
			}
			pending = map[*resource]bool{}
		}
	}
}

// markDirty decides which resources an event affects (CONS-VOL-6). An event
// naming a resource's own file or cache file marks only that resource. Any
// other event, including one without a path, marks every VOLUME resource.
func (c *Consumer) markDirty(pending map[*resource]bool, path string) {
	base := ""
	if path != "" {
		base = filepath.Base(path)
	}
	matched := false
	for _, r := range c.resources {
		switch r.mode {
		case opts.VOLUME:
			if base == "_"+r.name {
				pending[r] = true
				matched = true
			}
		case opts.DEV:
			if base != "" && base == filepath.Base(r.cachePath) {
				pending[r] = true
				matched = true
			}
		}
	}
	if matched {
		return
	}
	for _, r := range c.resources {
		if r.mode == opts.VOLUME {
			pending[r] = true
		}
	}
}
