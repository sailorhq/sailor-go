package sailor

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

// flakyTransport fails GET or POST requests on demand.
type flakyTransport struct {
	failGET  atomic.Bool
	failPOST atomic.Bool
	base     http.RoundTripper
}

func (f *flakyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if (req.Method == http.MethodGet && f.failGET.Load()) || (req.Method == http.MethodPost && f.failPOST.Load()) {
		return nil, errors.New("connection refused by test")
	}
	base := f.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func typedPull(name string) opts.ResourceOption {
	return Resource[map[string]any](opts.ResourceDefinition{Name: name}, opts.FetchDefinition{Fetch: opts.PULL, PullInterval: 10 * time.Second})
}

// ---- configuration validation ----

func TestValidationErrors(t *testing.T) {
	te := newTestEnv(t)
	conn := &opts.ConnectionOption{Addr: "h", Namespace: "n", App: "a"}
	cases := []struct {
		name string
		in   opts.InitOption
		want Code
	}{
		{"duplicate name", opts.InitOption{Connection: conn, Resources: []opts.ResourceOption{rawPull("x", time.Second), rawPull("x", time.Second)}}, CodeDuplicateResource},
		{"empty name", opts.InitOption{Connection: conn, Resources: []opts.ResourceOption{rawPull("", time.Second)}}, CodeEmptyResourceName},
		{"negative interval", opts.InitOption{Connection: conn, Resources: []opts.ResourceOption{rawPull("x", -time.Second)}}, CodeInvalidPullInterval},
		{"interval below minimum", opts.InitOption{Connection: conn, Resources: []opts.ResourceOption{rawPull("x", 500*time.Millisecond)}}, CodeInvalidPullInterval},
		{"no fetch mode", opts.InitOption{Connection: conn, Resources: []opts.ResourceOption{Raw(opts.ResourceDefinition{Name: "x"}, opts.FetchDefinition{})}}, CodeInvalidFetchMode},
		{"VOLUME without Path", opts.InitOption{Connection: conn, Resources: []opts.ResourceOption{volumeRes("x", "")}}, CodeInvalidFetchMode},
		{"fields connection without namespace", opts.InitOption{Connection: &opts.ConnectionOption{Addr: "h", App: "a"}, Resources: []opts.ResourceOption{rawPull("x", time.Second)}}, CodeMissingNamespaceApp},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newConsumer(tc.in, te.deps)
			var se *Error
			if !errors.As(err, &se) || se.Code != tc.want {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
		})
	}
}

func TestStartTwiceAndAfterClose(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("x"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)
	if err := c.Start(context.Background()); !errors.Is(err, &Error{Code: CodeAlreadyStarted}) {
		t.Fatalf("second Start = %v", err)
	}

	closed := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	_ = closed.Close()
	if err := closed.Start(context.Background()); !errors.Is(err, &Error{Code: CodeAlreadyStarted}) {
		t.Fatalf("Start after Close = %v", err)
	}
}

func TestCloseWithoutStartIsSafe(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestUnsupportedLangFailsAtDecode(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("cfg", fakeResource{body: []byte("a: 1"), version: "1"})
	res := Resource[map[string]any](opts.ResourceDefinition{Name: "cfg", Lang: "toml"}, opts.FetchDefinition{Fetch: opts.PULL, PullInterval: 10 * time.Second})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{res}, Connection: serverConn(fb, "")})
	if err := c.Start(context.Background()); !errors.Is(err, ErrUnsupportedLang) {
		t.Fatalf("Start = %v, want UNSUPPORTED_LANG", err)
	}
	_ = c.Close()
}

func TestStartFailureReturnsError(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("good", fakeResource{body: []byte("x"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("good", 10*time.Second), rawPull("missing", 10*time.Second)}, Connection: serverConn(fb, "")})
	if err := c.Start(context.Background()); !errors.Is(err, ErrResourceNotLoaded) {
		t.Fatalf("Start = %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

// ---- decode and transport failures ----

// A poll that brings broken content keeps the last value, logs decode_failed
// once for that content, and logs stale once.
func TestPollDecodeFailureLoggedOnce(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("cfg", fakeResource{body: []byte(`{"port":1}`), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{typedPull("cfg")}, Connection: serverConn(fb, "")})
	startConsumer(t, c)

	fb.set("cfg", fakeResource{body: []byte(`{"port":`), version: "2"})
	tick(t, te, 10*time.Second)
	te.clock.awaitWaiter(t, 10*time.Second)
	tick(t, te, 10*time.Second)
	te.clock.awaitWaiter(t, 10*time.Second)

	if n := te.log.count("resource.decode_failed"); n != 1 {
		t.Fatalf("decode_failed logged %d times, want 1", n)
	}
	if n := te.log.count("resource.stale"); n != 1 {
		t.Fatalf("stale logged %d times, want 1", n)
	}
	got, err := Get[map[string]any](c, "cfg")
	if err != nil || got["port"] != float64(1) {
		t.Fatalf("last good value lost: %v, %v", got, err)
	}
}

func TestPollTransportErrorIsStale(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("x"), version: "1"})
	ft := &flakyTransport{}
	c := te.newConsumer(t, opts.InitOption{
		Resources:  []opts.ResourceOption{rawPull("raw", 10*time.Second)},
		Connection: serverConn(fb, ""),
		HTTPClient: &http.Client{Transport: ft},
	})
	startConsumer(t, c)

	ft.failGET.Store(true)
	tick(t, te, 10*time.Second)
	ev := te.log.await(t, "resource.stale")
	if !strings.Contains(ev.kv["cause"].(string), "transport") {
		t.Fatalf("cause = %v", ev.kv["cause"])
	}
	te.clock.awaitWaiter(t, 10*time.Second)
}

func TestReportTransportErrorIsLogged(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("x"), version: "1"})
	ft := &flakyTransport{}
	ft.failPOST.Store(true)
	c := te.newConsumer(t, opts.InitOption{
		Resources:  []opts.ResourceOption{rawPull("raw", 10*time.Second)},
		Connection: serverConn(fb, "t"),
		HTTPClient: &http.Client{Transport: ft},
	})
	startConsumer(t, c)
	ev := te.log.await(t, "report.failed")
	if ev.kv["status"] != 0 {
		t.Fatalf("status = %v, want 0 for transport failure", ev.kv["status"])
	}
}

func TestFallbackUnreachableFails(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	te.env["SAILOR_FALLBACK_BASE_URL"] = "http://127.0.0.1:1/unreachable"
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{volumeRes("config", "/etc/sailor")}, Connection: serverConn(fb, "")})
	if err := c.Start(context.Background()); !errors.Is(err, ErrResourceNotLoaded) {
		t.Fatalf("Start = %v", err)
	}
	_ = c.Close()
}

func TestNewDefaultLoggerAndHelpers(t *testing.T) {
	l := newDefaultLogger()
	for _, lvl := range []opts.Level{opts.LevelDebug, opts.LevelInfo, opts.LevelWarn, opts.LevelError} {
		l.Log(lvl, "test.event", "k", "v")
	}
	if shortDigest("not-a-digest") != "not-a-digest" {
		t.Fatal("shortDigest changed a non-digest")
	}
	if got := shortDigest("sha256:0123456789abcdef0123"); got != "sha256:0123456789ab" {
		t.Fatalf("shortDigest = %q", got)
	}
	if redactURL("://bad") != "<unparseable>" {
		t.Fatal("redactURL did not handle a bad URL")
	}
	if truncate("abc", 10) != "abc" || truncate("abcdef", 3) != "abc" {
		t.Fatal("truncate is wrong")
	}
}

// ---- DEV failures and watching ----

func TestDevColdCacheServerDownFails(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("config", fakeResource{status: http.StatusInternalServerError})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{devRes("config", false)}, Connection: serverConn(fb, "")})
	if err := c.Start(context.Background()); !errors.Is(err, ErrResourceNotLoaded) {
		t.Fatalf("Start = %v", err)
	}
	if te.log.count("resource.load_failed") != 1 {
		t.Fatal("load failure not logged")
	}
	_ = c.Close()
}

func TestDevWatchLocalEditIsRead(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	seedPristineCache(te, "config", []byte("pristine"), "1")
	fb.set("config", fakeResource{body: []byte("pristine"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{devRes("config", false)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)

	te.fs.set(devCachePathFor("config"), []byte("hand edited"))
	te.watcher.events <- watchEvent{Path: devCachePathFor("config")}
	tick(t, te, watchDebounce)
	te.log.await(t, "resource.local_override")
	if got := getRaw(t, c, "config"); string(got) != "hand edited" {
		t.Fatalf("value = %q", got)
	}
}

func TestVolumeWatchUnmatchedEventRereadsAll(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	te.fs.set("/etc/sailor/_a", []byte("a1"))
	te.fs.set("/etc/sailor/_b", []byte("b1"))
	c := te.newConsumer(t, opts.InitOption{
		Resources:  []opts.ResourceOption{volumeRes("a", "/etc/sailor"), volumeRes("b", "/etc/sailor")},
		Connection: serverConn(fb, ""),
	})
	startConsumer(t, c)

	te.fs.set("/etc/sailor/_a", []byte("a2"))
	te.fs.set("/etc/sailor/_b", []byte("b2"))
	te.watcher.events <- watchEvent{Path: "/etc/sailor/..data"}
	tick(t, te, watchDebounce)
	te.log.await(t, "resource.changed")
	te.log.await(t, "resource.changed")
	if getRaw(t, c, "a")[1] != '2' || getRaw(t, c, "b")[1] != '2' {
		t.Fatal("unmatched event did not reread every volume resource")
	}
}

func TestVolumeWatchDecodeFailureIsLogged(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	te.fs.set("/etc/sailor/_cfg", []byte(`{"ok":true}`))
	res := Resource[map[string]any](opts.ResourceDefinition{Name: "cfg", Path: "/etc/sailor"}, opts.FetchDefinition{Fetch: opts.VOLUME})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{res}, Connection: serverConn(fb, "")})
	startConsumer(t, c)

	te.fs.set("/etc/sailor/_cfg", []byte(`{broken`))
	te.watcher.events <- watchEvent{Path: "/etc/sailor/_cfg"}
	tick(t, te, watchDebounce)
	te.log.await(t, "resource.decode_failed")
	if got, err := Get[map[string]any](c, "cfg"); err != nil || got["ok"] != true {
		t.Fatalf("last good value lost: %v, %v", got, err)
	}
}

func TestWatcherFailuresAreLogged(t *testing.T) {
	te := newTestEnv(t)
	te.deps.newWatcher = func() (watcher, error) { return nil, errors.New("no inotify") }
	fb := newFakeBackend(t)
	te.fs.set("/etc/sailor/_config", []byte("x"))
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{volumeRes("config", "/etc/sailor")}, Connection: serverConn(fb, "")})
	startConsumer(t, c)
	if te.log.count("watcher.failed") != 1 {
		t.Fatal("watcher construction failure not logged")
	}
}

func TestWatcherAddAndErrorsAreLogged(t *testing.T) {
	te := newTestEnv(t)
	te.watcher.addErr = errors.New("cannot watch")
	fb := newFakeBackend(t)
	te.fs.set("/etc/sailor/_config", []byte("x"))
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{volumeRes("config", "/etc/sailor")}, Connection: serverConn(fb, "")})
	startConsumer(t, c)
	te.log.await(t, "watcher.failed")

	te.watcher.errs <- errors.New("queue overflow")
	ev := te.log.awaitWhere(t, "watcher.failed", func(e loggedEvent) bool {
		s, _ := e.kv["cause"].(string)
		return strings.Contains(s, "overflow")
	})
	if ev.level != opts.LevelError {
		t.Fatalf("level = %v", ev.level)
	}
}

func TestWatchDisabledCreatesNoWatcher(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	te.fs.set("/etc/sailor/_config", []byte("x"))
	c := te.newConsumer(t, opts.InitOption{
		Resources:  []opts.ResourceOption{volumeRes("config", "/etc/sailor")},
		Connection: serverConn(fb, ""),
		Watch:      boolPtr(false),
	})
	startConsumer(t, c)
	if dirs := te.watcher.watchedDirs(); len(dirs) != 0 {
		t.Fatalf("watched %v with Watch=false", dirs)
	}
}

func TestDevCachePathUsesNamespaceAppEnvName(t *testing.T) {
	te := newTestEnv(t)
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{devRes("config", false)}, Connection: serverConn(newFakeBackend(t), "")})
	p, err := c.devCachePath(c.byName["config"])
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/home/test", ".sailor", "cache", "n-a-test-config.json"); p != want {
		t.Fatalf("path = %q, want %q", p, want)
	}
}

// ---- pure helpers ----

func TestStoreSourceUnchangedWithoutDigestChange(t *testing.T) {
	te := newTestEnv(t)
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(newFakeBackend(t), "")})
	r := c.byName["raw"]
	now := te.clock.Now()
	first, _ := c.storeSource(r, []byte("same"), "1", now)
	second, _ := c.storeSource(r, []byte("same"), "1", now)
	if !first || second {
		t.Fatalf("changed flags = %v, %v; want true, false", first, second)
	}
}

func TestStoreSourceEncryptedFailureKeepsState(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	res := Resource[map[string]string](opts.ResourceDefinition{Name: "s", Encrypted: true}, opts.FetchDefinition{Fetch: opts.PULL, PullInterval: 10 * time.Second})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{res}, Connection: &opts.ConnectionOption{Addr: fb.URL(), Namespace: "n", App: "a", AccessKey: "ak", SecretKey: "sk"}})
	r := c.byName["s"]
	if _, err := c.storeSource(r, []byte(`{"bad":`), "1", te.clock.Now()); err == nil {
		t.Fatal("expected a decode error")
	}
	if loaded, _, _ := r.snapshot(); loaded {
		t.Fatal("failed store changed the state")
	}
}
