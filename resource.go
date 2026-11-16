package sailor

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"sync"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

// resource is one managed resource and its state (SAILOR_CONSUMER.md section 8).
// mu guards every field below it.
type resource struct {
	name      string
	mode      opts.FetchOption
	once      bool
	interval  time.Duration
	encrypted bool
	dir       string // VOLUME mount directory, or DEV cache directory
	cachePath string // DEV only: the cache file, set during Start
	lang      string // normalised; always "json" when encrypted (CONS-RES-9)
	decode    func(raw []byte, decode opts.DecodeFunc) (any, error)

	mu             sync.Mutex
	loaded         bool
	value          any
	version        string
	digest         string
	stale          bool
	staleCause     string
	lastSuccess    time.Time
	noTokenLogged  bool
	lastBadDigest  string
	mismatchLogged bool
}

func (r *resource) volumeFile() string {
	return filepath.Join(r.dir, "_"+r.name)
}

func modeName(m opts.FetchOption) string {
	switch m {
	case opts.VOLUME:
		return "volume"
	case opts.PULL:
		return "pull"
	case opts.DEV:
		return "dev"
	}
	return "unknown"
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// storeSource is the single state-recording step (CONS-STATE-5). It returns
// whether the digest of the bytes as received differs from the stored one.
// On any error the previous state is untouched.
func (c *Consumer) storeSource(r *resource, src []byte, version string, now time.Time) (bool, error) {
	digest := digestOf(src)

	plain := src
	if r.encrypted {
		var err error
		plain, err = decryptResource(src, c.conn.accessKey, c.conn.secretKey)
		if err != nil {
			return false, err
		}
	}

	value, err := c.buildValue(r, plain)
	if err != nil {
		return false, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	changed := !r.loaded || r.digest != digest
	r.value = value
	r.digest = digest
	if version != "" {
		r.version = version
	}
	r.loaded = true
	r.lastSuccess = now
	return changed, nil
}

// buildValue decodes resolved bytes. Raw resources store a copy of the bytes.
func (c *Consumer) buildValue(r *resource, plain []byte) (any, error) {
	if r.decode == nil {
		return append([]byte(nil), plain...), nil
	}
	fn, err := c.decoders.lookup(r.lang)
	if err != nil {
		return nil, err
	}
	v, err := r.decode(plain, fn)
	if err != nil {
		return nil, wrapf(CodeDecodeFailed, err, "decoding resource %q", r.name)
	}
	return v, nil
}

// markStale records a refresh failure after a success, logging once per
// transition (CONS-PULL-6, CONS-LOG-5). It does nothing before the first load.
func (c *Consumer) markStale(r *resource, cause string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.loaded || r.stale {
		return
	}
	r.stale = true
	r.staleCause = cause
	c.emit(opts.LevelWarn, "resource.stale",
		"resource", r.name,
		"mode", modeName(r.mode),
		"cause", cause,
		"version", r.version,
		"seconds_since_success", int(c.clock.Now().Sub(r.lastSuccess).Seconds()),
	)
}

// markRecovered clears a stale condition, logging once (CONS-PULL-6).
func (c *Consumer) markRecovered(r *resource, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastSuccess = now
	if !r.stale {
		return
	}
	r.stale = false
	r.staleCause = ""
	c.emit(opts.LevelInfo, "resource.recovered", "resource", r.name, "version", r.version)
}

// knownVersion returns the version to send as If-None-Match, or "".
func (r *resource) knownVersion() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.version
}

// snapshot returns the state a heartbeat or report needs.
func (r *resource) snapshot() (loaded bool, version, digest string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loaded, r.version, r.digest
}
