package sailor

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

// ---- fake clock ----

type fakeClock struct {
	mu         sync.Mutex
	now        time.Time
	waiters    []*fakeWaiter
	registered chan time.Duration
}

type fakeWaiter struct {
	d  time.Duration
	ch chan time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(1_700_000_000, 0), registered: make(chan time.Duration, 1000)}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) After(d time.Duration) <-chan time.Time {
	f.mu.Lock()
	w := &fakeWaiter{d: d, ch: make(chan time.Time, 1)}
	f.waiters = append(f.waiters, w)
	f.mu.Unlock()
	f.registered <- d
	return w.ch
}

// fire releases every waiter registered with duration d.
func (f *fakeClock) fire(d time.Duration) {
	f.mu.Lock()
	var keep []*fakeWaiter
	var fired []*fakeWaiter
	for _, w := range f.waiters {
		if w.d == d {
			fired = append(fired, w)
		} else {
			keep = append(keep, w)
		}
	}
	f.waiters = keep
	now := f.now
	f.mu.Unlock()
	for _, w := range fired {
		w.ch <- now
	}
}

func (f *fakeClock) hasWaiter(d time.Duration) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.waiters {
		if w.d == d {
			return true
		}
	}
	return false
}

// awaitWaiter blocks until a pending timer of duration d exists. It does not
// consume the timer, so a later fire can still release it.
func (f *fakeClock) awaitWaiter(t *testing.T, d time.Duration) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		f.mu.Lock()
		for _, w := range f.waiters {
			if w.d == d {
				f.mu.Unlock()
				return
			}
		}
		f.mu.Unlock()
		select {
		case <-f.registered:
		case <-timeout:
			t.Fatalf("timed out waiting for a timer of %s", d)
		}
	}
}

// ---- in-memory filesystem ----

type memFS struct {
	mu    sync.Mutex
	files map[string][]byte
	dirs  map[string]bool
}

func newMemFS() *memFS { return &memFS{files: map[string][]byte{}, dirs: map[string]bool{}} }

func (m *memFS) ReadFile(path string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[path]
	if !ok {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	return append([]byte(nil), b...), nil
}

func (m *memFS) WriteFile(path string, data []byte, _ os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = append([]byte(nil), data...)
	return nil
}

func (m *memFS) MkdirAll(path string, _ os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirs[path] = true
	return nil
}

func (m *memFS) set(path string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = append([]byte(nil), data...)
}

func (m *memFS) get(path string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[path]
	return append([]byte(nil), b...), ok
}

// ---- fake watcher ----

type fakeWatcher struct {
	mu     sync.Mutex
	dirs   []string
	events chan watchEvent
	errs   chan error
	closed bool
	addErr error
}

func newFakeWatcher() *fakeWatcher {
	return &fakeWatcher{events: make(chan watchEvent, 100), errs: make(chan error, 10)}
}

func (w *fakeWatcher) Add(dir string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.addErr != nil {
		return w.addErr
	}
	w.dirs = append(w.dirs, dir)
	return nil
}
func (w *fakeWatcher) Events() <-chan watchEvent { return w.events }
func (w *fakeWatcher) Errors() <-chan error      { return w.errs }
func (w *fakeWatcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

func (w *fakeWatcher) watchedDirs() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.dirs...)
}

// ---- recording logger ----

type loggedEvent struct {
	level opts.Level
	name  string
	kv    map[string]any
}

type recordingLogger struct {
	mu     sync.Mutex
	events []loggedEvent
	ch     chan loggedEvent
}

func newRecordingLogger() *recordingLogger {
	return &recordingLogger{ch: make(chan loggedEvent, 1000)}
}

func (r *recordingLogger) Log(level opts.Level, event string, kv ...any) {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	e := loggedEvent{level: level, name: event, kv: m}
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
	r.ch <- e
}

// count returns how many logged events have the given name.
func (r *recordingLogger) count(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if e.name == name {
			n++
		}
	}
	return n
}

func (r *recordingLogger) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		out = append(out, e.name)
	}
	return out
}

// awaitWhere blocks until an event with the given name satisfies match.
func (r *recordingLogger) awaitWhere(t *testing.T, name string, match func(loggedEvent) bool) loggedEvent {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		r.mu.Lock()
		for _, e := range r.events {
			if e.name == name && match(e) {
				r.mu.Unlock()
				return e
			}
		}
		r.mu.Unlock()
		select {
		case <-r.ch:
		case <-timeout:
			t.Fatalf("timed out waiting for event %q matching condition; saw %v", name, r.names())
		}
	}
}

// await blocks until an event with the given name is logged.
func (r *recordingLogger) await(t *testing.T, name string) loggedEvent {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		if r.count(name) > 0 {
			r.mu.Lock()
			defer r.mu.Unlock()
			for _, e := range r.events {
				if e.name == name {
					return e
				}
			}
		}
		select {
		case <-r.ch:
		case <-timeout:
			t.Fatalf("timed out waiting for event %q; saw %v", name, r.names())
		}
	}
}

// ---- fake backend ----

type fakeResource struct {
	status    int
	body      []byte
	version   string // sent as x-resource-version
	etag      string // sent as ETag when non-empty
	encrypted bool
	// ignoreConditional makes the backend answer 200 even when If-None-Match matches.
	ignoreConditional bool
}

type fakeReport struct {
	Consumer string            `json:"consumer"`
	Version  string            `json:"version"`
	Digest   string            `json:"digest"`
	Details  map[string]string `json:"details"`
}

type fakeRequest struct {
	method      string
	path        string
	token       string
	ifNoneMatch string
}

type fakeBackend struct {
	t            *testing.T
	mu           sync.Mutex
	srv          *httptest.Server
	res          map[string]fakeResource
	files        map[string]fakeResource // static paths, e.g. fallback blobs
	reqs         []fakeRequest
	reports      []fakeReport
	reportStatus int
	reportSignal chan struct{}
}

func newFakeBackend(t *testing.T) *fakeBackend {
	t.Helper()
	fb := &fakeBackend{
		t:            t,
		res:          map[string]fakeResource{},
		files:        map[string]fakeResource{},
		reportStatus: http.StatusOK,
		reportSignal: make(chan struct{}, 1000),
	}
	fb.srv = httptest.NewServer(http.HandlerFunc(fb.serve))
	t.Cleanup(fb.srv.Close)
	return fb
}

func (fb *fakeBackend) URL() string { return fb.srv.URL }

func (fb *fakeBackend) set(name string, r fakeResource) {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.res[name] = r
}

// setFile serves a static path, such as a fallback blob.
func (fb *fakeBackend) setFile(path string, r fakeResource) {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.files[path] = r
}

func writeFake(w http.ResponseWriter, f fakeResource) {
	status := f.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(f.body)
}

func (fb *fakeBackend) requests() []fakeRequest {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	return append([]fakeRequest(nil), fb.reqs...)
}

func (fb *fakeBackend) reportList() []fakeReport {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	return append([]fakeReport(nil), fb.reports...)
}

func (fb *fakeBackend) serve(w http.ResponseWriter, r *http.Request) {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.reqs = append(fb.reqs, fakeRequest{
		method:      r.Method,
		path:        r.URL.Path,
		token:       r.Header.Get("x-token"),
		ifNoneMatch: r.Header.Get("If-None-Match"),
	})

	if f, ok := fb.files[r.URL.Path]; ok {
		writeFake(w, f)
		return
	}

	// /api/v1/resource/{ns}/{app}/{name}[/consumer-report]
	parts := splitPath(r.URL.Path)
	if len(parts) < 6 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "resource" {
		http.NotFound(w, r)
		return
	}
	name := parts[5]

	if len(parts) == 7 && parts[6] == "consumer-report" && r.Method == http.MethodPost {
		body, _ := io.ReadAll(r.Body)
		var rep fakeReport
		if err := json.Unmarshal(body, &rep); err != nil || rep.Consumer == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"consumer is required"}` + "\n"))
			return
		}
		fb.reports = append(fb.reports, rep)
		fb.reportSignal <- struct{}{}
		if fb.reportStatus != http.StatusOK {
			w.WriteHeader(fb.reportStatus)
			_, _ = w.Write([]byte(`{"message":"report rejected"}` + "\n"))
			return
		}
		_, _ = w.Write([]byte(`{"message":"ok"}` + "\n"))
		return
	}

	res, ok := fb.res[name]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"this resource was never deployed!"}` + "\n"))
		return
	}
	if res.encrypted {
		w.Header().Set("x-resource-encrypted", "true")
	} else {
		w.Header().Set("x-resource-encrypted", "false")
	}
	if res.status != 0 && res.status != http.StatusOK {
		w.WriteHeader(res.status)
		_, _ = w.Write([]byte(`{"message":"` + http.StatusText(res.status) + `"}` + "\n"))
		return
	}
	if res.version != "" {
		w.Header().Set("x-resource-version", res.version)
	}
	if res.etag != "" {
		w.Header().Set("ETag", res.etag)
		if !res.ignoreConditional && r.Header.Get("If-None-Match") == res.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	_, _ = w.Write(res.body)
}

func splitPath(p string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(p); i++ {
		if i == len(p) || p[i] == '/' {
			if i > start {
				out = append(out, p[start:i])
			}
			start = i + 1
		}
	}
	return out
}

// ---- test dependencies ----

type testEnv struct {
	clock   *fakeClock
	fs      *memFS
	watcher *fakeWatcher
	log     *recordingLogger
	env     map[string]string
	deps    deps
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	te := &testEnv{
		clock:   newFakeClock(),
		fs:      newMemFS(),
		watcher: newFakeWatcher(),
		log:     newRecordingLogger(),
		env:     map[string]string{},
	}
	te.deps = deps{
		clock:    te.clock,
		fs:       te.fs,
		getenv:   func(k string) string { return te.env[k] },
		homeDir:  func() (string, error) { return "/home/test", nil },
		hostname: func() (string, error) { return "test-host", nil },
		newWatcher: func() (watcher, error) {
			return te.watcher, nil
		},
	}
	return te
}

func (te *testEnv) newConsumer(t *testing.T, in opts.InitOption) *Consumer {
	t.Helper()
	in.Logger = te.log
	c, err := newConsumer(in, te.deps)
	if err != nil {
		t.Fatalf("newConsumer: %v", err)
	}
	return c
}

func boolPtr(b bool) *bool { return &b }

// ---- crypto (test-only encoder, mirrors the backend's layout) ----

func testSeal(key, plain []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}
	return append(nonce, gcm.Seal(nil, nonce, plain, nil)...)
}

// testEncryptedBody builds the encrypted wire body for the given values.
func testEncryptedBody(t *testing.T, accessKey, secretKey string, values map[string]string) []byte {
	t.Helper()
	kek, err := deriveKEK(secretKey, accessKey)
	if err != nil {
		t.Fatalf("deriveKEK: %v", err)
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := map[string]secretRecord{}
	for _, k := range keys {
		dek := make([]byte, dekSize)
		if _, err := rand.Read(dek); err != nil {
			t.Fatal(err)
		}
		out[k] = secretRecord{
			EncryptedSecret: base64.StdEncoding.EncodeToString(testSeal(dek, []byte(values[k]))),
			EncryptedDEK:    base64.StdEncoding.EncodeToString(testSeal(kek, dek)),
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sha256Sum(b []byte) []byte {
	s := sha256.Sum256(b)
	return s[:]
}
