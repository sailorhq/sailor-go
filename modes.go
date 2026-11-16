package sailor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

// devMeta is the DEV cache sidecar (CONS-DEV-2).
type devMeta struct {
	Version string `json:"version"`
	Hash    string `json:"hash"`
}

func hexSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// versionDigest reads the state needed to decide what changed.
func (r *resource) versionDigest() (version, digest string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.version, r.digest
}

// apply records bytes from a source and emits either resource.loaded (first
// value) or resource.changed (digest differs), then reports a change
// (CONS-STATE-5, CONS-STATE-6).
func (c *Consumer) apply(r *resource, body []byte, version, source string, now time.Time) error {
	oldVersion, oldDigest := r.versionDigest()
	changed, err := c.storeSource(r, body, version, now)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	_, newVersion, newDigest := r.snapshot()
	if oldDigest == "" {
		c.emit(opts.LevelInfo, "resource.loaded",
			"resource", r.name, "mode", modeName(r.mode), "source", source,
			"version", newVersion, "digest", shortDigest(newDigest))
	} else {
		c.emit(opts.LevelInfo, "resource.changed",
			"resource", r.name, "mode", modeName(r.mode), "source", source,
			"old_version", oldVersion, "new_version", newVersion,
			"old_digest", shortDigest(oldDigest), "new_digest", shortDigest(newDigest))
	}
	c.reportAsync(r, newVersion, newDigest)
	return nil
}

// failLoad logs a load that will fail Start and returns err.
func (c *Consumer) failLoad(r *resource, err error) error {
	c.emit(opts.LevelError, "resource.load_failed",
		"resource", r.name, "mode", modeName(r.mode), "cause", err.Error())
	return err
}

// loadInitial loads a resource during Start.
func (c *Consumer) loadInitial(ctx context.Context, r *resource) error {
	now := c.clock.Now()
	switch r.mode {
	case opts.VOLUME:
		return c.loadVolume(ctx, r, now)
	case opts.PULL:
		return c.loadPull(ctx, r, now)
	case opts.DEV:
		return c.loadDev(ctx, r, now)
	}
	return errf(CodeInvalidFetchMode, "resource %q has an unknown fetch mode", r.name)
}

func (c *Consumer) loadVolume(ctx context.Context, r *resource, now time.Time) error {
	data, err := c.deps.fs.ReadFile(r.volumeFile())
	if err != nil {
		return c.loadFromFallback(ctx, r, now, "volume file unreadable: "+err.Error())
	}
	if err := c.apply(r, data, "", "volume", now); err != nil {
		return c.failLoad(r, err)
	}
	return nil
}

func (c *Consumer) loadPull(ctx context.Context, r *resource, now time.Time) error {
	resp, err := c.fetchResource(ctx, r, "")
	if err != nil {
		return c.loadFromFallback(ctx, r, now, "transport: "+err.Error())
	}
	if resp.status != 200 {
		return c.loadFromFallback(ctx, r, now, fmt.Sprintf("HTTP %d", resp.status))
	}
	if err := c.apply(r, resp.body, versionOf(resp.header), "server", now); err != nil {
		return c.failLoad(r, err)
	}
	return nil
}

// loadFromFallback is the fallback step for VOLUME and PULL (CONS-FB-5, CONS-FB-7).
func (c *Consumer) loadFromFallback(ctx context.Context, r *resource, now time.Time, reason string) error {
	body, target, err := c.fetchFallback(ctx, r)
	if err != nil {
		cause := reason + "; fallback: " + err.Error()
		c.emit(opts.LevelError, "resource.load_failed",
			"resource", r.name, "mode", modeName(r.mode), "cause", cause)
		return wrapf(CodeResourceNotLoaded, err, "resource %q has no value", r.name)
	}
	c.emit(opts.LevelWarn, "resource.fallback_used",
		"resource", r.name, "reason", reason, "url", redactURL(target))
	if err := c.apply(r, body, "", "fallback", now); err != nil {
		return c.failLoad(r, err)
	}
	return nil
}

// devCachePath is {home}/.sailor/cache/{ns}-{app}-{env}-{name}.json (CONS-DEV-1).
func (c *Consumer) devCachePath(r *resource) (string, error) {
	home, err := c.deps.homeDir()
	if err != nil {
		return "", wrapf(CodeResourceNotLoaded, err, "finding home directory for DEV cache")
	}
	name := fmt.Sprintf("%s-%s-%s-%s.json", c.conn.namespace, c.conn.app, c.conn.env, r.name)
	return filepath.Join(home, ".sailor", "cache", name), nil
}

func (c *Consumer) readDevMeta(cachePath string) (devMeta, bool) {
	var m devMeta
	data, err := c.deps.fs.ReadFile(cachePath + ".meta.json")
	if err != nil {
		return m, false
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, false
	}
	return m, true
}

// writeDevCache stores upstream bytes and their sidecar (CONS-DEV-5).
func (c *Consumer) writeDevCache(cachePath string, body []byte, version string) error {
	if err := c.deps.fs.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return err
	}
	if err := c.deps.fs.WriteFile(cachePath, body, 0o644); err != nil {
		return err
	}
	meta, err := json.Marshal(devMeta{Version: version, Hash: hexSHA256(body)})
	if err != nil {
		return err
	}
	return c.deps.fs.WriteFile(cachePath+".meta.json", meta, 0o644)
}

// loadDev implements the CONS-DEV-4 decision table at start.
func (c *Consumer) loadDev(ctx context.Context, r *resource, now time.Time) error {
	cachePath, err := c.devCachePath(r)
	if err != nil {
		return c.failLoad(r, err)
	}
	r.cachePath = cachePath
	r.dir = filepath.Dir(cachePath)

	cached, readErr := c.deps.fs.ReadFile(cachePath)
	if readErr != nil {
		return c.devFetchCold(ctx, r, cachePath, now)
	}

	meta, hasMeta := c.readDevMeta(cachePath)
	if !hasMeta || meta.Hash != hexSHA256(cached) {
		c.emit(opts.LevelInfo, "resource.local_override", "resource", r.name, "path", cachePath)
		return c.apply(r, cached, "", "local_override", now)
	}

	resp, err := c.fetchResource(ctx, r, meta.Version)
	switch {
	case err != nil:
		return c.devUseCached(r, cached, meta.Version, now, "transport: "+err.Error())
	case resp.status == 304:
		return c.apply(r, cached, meta.Version, "cache", now)
	case resp.status == 200:
		version := versionOf(resp.header)
		if hexSHA256(resp.body) == meta.Hash {
			return c.apply(r, cached, version, "cache", now)
		}
		if err := c.writeDevCache(cachePath, resp.body, version); err != nil {
			return c.failLoad(r, err)
		}
		return c.apply(r, resp.body, version, "server", now)
	default:
		return c.devUseCached(r, cached, meta.Version, now, fmt.Sprintf("HTTP %d", resp.status))
	}
}

// devFetchCold handles a DEV resource with no cache file yet.
func (c *Consumer) devFetchCold(ctx context.Context, r *resource, cachePath string, now time.Time) error {
	resp, err := c.fetchResource(ctx, r, "")
	if err != nil {
		return c.failLoad(r, wrapf(CodeResourceNotLoaded, err, "no cache and server unreachable"))
	}
	if resp.status != 200 {
		return c.failLoad(r, errf(CodeResourceNotLoaded, "no cache and server answered HTTP %d", resp.status))
	}
	version := versionOf(resp.header)
	if err := c.writeDevCache(cachePath, resp.body, version); err != nil {
		return c.failLoad(r, err)
	}
	return c.apply(r, resp.body, version, "server", now)
}

// devUseCached keeps a pristine cache when the server cannot be asked (CONS-DEV-4).
func (c *Consumer) devUseCached(r *resource, cached []byte, version string, now time.Time, cause string) error {
	if err := c.apply(r, cached, version, "cache", now); err != nil {
		return c.failLoad(r, err)
	}
	c.markStale(r, cause)
	return nil
}

// pollLoop is the single goroutine that refreshes one PULL resource (CONS-PULL-3).
func (c *Consumer) pollLoop(r *resource) {
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.clock.After(r.interval):
		}
		c.pollOnce(r)
	}
}

func (c *Consumer) pollOnce(r *resource) {
	now := c.clock.Now()
	resp, err := c.fetchResource(c.ctx, r, r.knownVersion())
	if err != nil {
		if c.ctx.Err() == nil {
			c.markStale(r, "transport: "+err.Error())
		}
		return
	}
	switch resp.status {
	case 200:
		if err := c.apply(r, resp.body, versionOf(resp.header), "server", now); err != nil {
			c.noteDecodeFailure(r, resp.body, err)
			c.markStale(r, "decode: "+err.Error())
			return
		}
		c.markRecovered(r, now)
	case 304:
		c.markRecovered(r, now)
	default:
		c.markStale(r, fmt.Sprintf("HTTP %d", resp.status))
	}
}

// noteDecodeFailure logs a bad payload once per distinct bad digest, so a
// server that keeps serving the same broken content does not flood the log.
func (c *Consumer) noteDecodeFailure(r *resource, body []byte, err error) {
	digest := digestOf(body)
	r.mu.Lock()
	if r.lastBadDigest == digest {
		r.mu.Unlock()
		return
	}
	r.lastBadDigest = digest
	r.mu.Unlock()
	c.emit(opts.LevelError, "resource.decode_failed",
		"resource", r.name, "digest", shortDigest(digest), "cause", err.Error())
}

// reread refreshes a VOLUME or DEV resource after a file event (CONS-VOL-6, CONS-DEV-6).
func (c *Consumer) reread(r *resource) {
	now := c.clock.Now()
	switch r.mode {
	case opts.VOLUME:
		data, err := c.deps.fs.ReadFile(r.volumeFile())
		if err != nil {
			c.markStale(r, "read: "+err.Error())
			return
		}
		if err := c.apply(r, data, "", "volume", now); err != nil {
			c.noteDecodeFailure(r, data, err)
			c.markStale(r, "decode: "+err.Error())
			return
		}
		c.markRecovered(r, now)
	case opts.DEV:
		data, err := c.deps.fs.ReadFile(r.cachePath)
		if err != nil {
			c.markStale(r, "read: "+err.Error())
			return
		}
		meta, hasMeta := c.readDevMeta(r.cachePath)
		if !hasMeta || meta.Hash != hexSHA256(data) {
			c.emit(opts.LevelInfo, "resource.local_override", "resource", r.name, "path", r.cachePath)
		}
		if err := c.apply(r, data, "", "cache", now); err != nil {
			c.noteDecodeFailure(r, data, err)
			c.markStale(r, "decode: "+err.Error())
			return
		}
		c.markRecovered(r, now)
	}
}
