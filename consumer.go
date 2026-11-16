package sailor

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

const (
	defaultPullInterval   = 10 * time.Second
	minPullInterval       = time.Second
	defaultRequestTimeout = 10 * time.Second
	defaultReportTimeout  = 5 * time.Second
	heartbeatInterval     = 10 * time.Minute
	watchDebounce         = 500 * time.Millisecond
)

// Consumer keeps a set of named resources current in memory. Construct it
// with NewConsumer, call Start, read with Get, and call Close when done.
type Consumer struct {
	conn      connection
	resources []*resource
	byName    map[string]*resource
	decoders  decoderSet
	watch     bool

	requestTimeout time.Duration
	reportTimeout  time.Duration

	log    opts.Logger
	http   *http.Client
	deps   deps
	clock  clock
	getenv func(string) string

	// lifecycle state, set by Start, read by Close
	mu        sync.Mutex
	started   bool
	closed    bool
	startedAt time.Time
	cancel    context.CancelFunc
	ctx       context.Context
	watcher   watcher
	wg        sync.WaitGroup
}

// NewConsumer validates the options and resolves the connection. It starts
// no goroutines and performs no I/O beyond reading ~/.sailor/config when
// UseLocalConfig is set (SAILOR_CONSUMER.md CONS-CONC-2).
func NewConsumer(in opts.InitOption) (*Consumer, error) {
	return newConsumer(in, defaultDeps())
}

func newConsumer(in opts.InitOption, d deps) (*Consumer, error) {
	if len(in.Resources) == 0 {
		return nil, ErrEmptyResources
	}

	byName := make(map[string]*resource, len(in.Resources))
	ordered := make([]*resource, 0, len(in.Resources))
	decoders := newDecoderSet(in.Decoders)
	for _, ro := range in.Resources {
		r, err := newResource(ro)
		if err != nil {
			return nil, err
		}
		if _, dup := byName[r.name]; dup {
			return nil, errf(CodeDuplicateResource, "resource %q is declared twice", r.name)
		}
		byName[r.name] = r
		ordered = append(ordered, r)
	}

	conn, err := resolveConnection(in, d)
	if err != nil {
		return nil, err
	}
	if conn.env == "" && in.Connection != nil {
		conn.env = in.Connection.Env
	}

	logger := in.Logger
	if logger == nil {
		logger = newDefaultLogger()
	}
	httpClient := in.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	requestTimeout := in.RequestTimeout
	if requestTimeout == 0 {
		requestTimeout = defaultRequestTimeout
	}
	reportTimeout := in.ReportTimeout
	if reportTimeout == 0 {
		reportTimeout = defaultReportTimeout
	}
	watch := true
	if in.Watch != nil {
		watch = *in.Watch
	}

	c := &Consumer{
		conn:           conn,
		resources:      ordered,
		byName:         byName,
		decoders:       decoders,
		watch:          watch,
		requestTimeout: requestTimeout,
		reportTimeout:  reportTimeout,
		log:            logger,
		http:           httpClient,
		deps:           d,
		clock:          d.clock,
		getenv:         d.getenv,
	}

	if conn.token != "" && strings.HasPrefix(conn.addr, "http://") && !isLoopbackAddr(conn.addr) {
		c.emit(opts.LevelWarn, "config.insecure_transport", "host", redactURL(conn.addr))
	}
	c.emit(opts.LevelInfo, "config.resolved",
		"source", conn.source,
		"addr", redactURL(conn.addr),
		"namespace", conn.namespace,
		"app", conn.app,
		"token", conn.token != "",
		"resources", len(ordered),
	)
	return c, nil
}

func newResource(ro opts.ResourceOption) (*resource, error) {
	if ro.Def.Name == "" {
		return nil, ErrEmptyResourceName
	}
	interval := ro.FetchDef.PullInterval
	switch {
	case interval < 0:
		return nil, errf(CodeInvalidPullInterval, "resource %q has a negative PullInterval", ro.Def.Name)
	case interval == 0:
		interval = defaultPullInterval
	case interval < minPullInterval:
		return nil, errf(CodeInvalidPullInterval, "resource %q PullInterval is below %s", ro.Def.Name, minPullInterval)
	}
	switch ro.FetchDef.Fetch {
	case opts.VOLUME:
		if ro.Def.Path == "" {
			return nil, errf(CodeInvalidFetchMode, "resource %q is VOLUME but has no Path", ro.Def.Name)
		}
	case opts.PULL, opts.DEV:
	default:
		return nil, errf(CodeInvalidFetchMode, "resource %q has no valid fetch mode", ro.Def.Name)
	}
	lang := normalizeLang(ro.Def.Lang)
	if ro.Def.Encrypted {
		lang = "json"
	}
	return &resource{
		name:      ro.Def.Name,
		mode:      ro.FetchDef.Fetch,
		once:      ro.FetchDef.Once,
		interval:  interval,
		encrypted: ro.Def.Encrypted,
		dir:       ro.Def.Path,
		lang:      lang,
		decode:    ro.Decode,
	}, nil
}

// Start loads every resource once and then starts background refresh. It
// fails if any resource cannot be loaded and has no fallback (CONS-CONC-3).
func (c *Consumer) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return errf(CodeAlreadyStarted, "Start has already been called")
	}
	if c.closed {
		c.mu.Unlock()
		return errf(CodeAlreadyStarted, "consumer is closed")
	}
	c.started = true
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.startedAt = c.clock.Now()
	c.mu.Unlock()

	if c.conn.token == "" {
		for _, r := range c.resources {
			c.logNoToken(r)
		}
	}

	for _, r := range c.resources {
		if err := c.loadInitial(ctx, r); err != nil {
			c.cancel()
			return err
		}
	}

	c.startWatcher()

	for _, r := range c.resources {
		if r.mode == opts.PULL && !r.once {
			c.spawn(func() { c.pollLoop(r) })
		}
		c.spawn(func() { c.heartbeatLoop(r) })
	}

	c.emit(opts.LevelInfo, "consumer.started", "resources", len(c.resources))
	return nil
}

// spawn runs f in a goroutine the consumer waits for on Close.
func (c *Consumer) spawn(f func()) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		f()
	}()
}

// Close stops every background goroutine and waits for them. It is idempotent.
func (c *Consumer) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	cancel := c.cancel
	watcher := c.watcher
	started := c.started
	startedAt := c.startedAt
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if watcher != nil {
		_ = watcher.Close()
	}
	c.wg.Wait()
	if started {
		c.emit(opts.LevelInfo, "consumer.stopped",
			"resources", len(c.resources),
			"uptime_seconds", int(c.clock.Now().Sub(startedAt).Seconds()),
		)
	}
	return nil
}

// Get returns the current value of a resource, typed as T (CONS-CONC-9, CONS-CONC-10).
func Get[T any](c *Consumer, name string) (T, error) {
	var zero T
	r, ok := c.byName[name]
	if !ok {
		return zero, errf(CodeResourceNotLoaded, "no resource named %q is declared", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.loaded {
		return zero, errf(CodeResourceNotLoaded, "resource %q has no value yet", name)
	}
	v, ok := r.value.(T)
	if !ok {
		return zero, errf(CodeTypeMismatch, "resource %q holds %T, not %T", name, r.value, zero)
	}
	return v, nil
}

func (c *Consumer) emit(level opts.Level, event string, kv ...any) {
	c.log.Log(level, event, kv...)
}

// logNoToken emits report.skipped_no_token once per resource (CONS-RPT-8).
func (c *Consumer) logNoToken(r *resource) {
	r.mu.Lock()
	already := r.noTokenLogged
	r.noTokenLogged = true
	r.mu.Unlock()
	if !already {
		c.emit(opts.LevelWarn, "report.skipped_no_token", "resource", r.name)
	}
}
