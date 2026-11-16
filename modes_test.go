package sailor

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

const testEnvName = "test"

// rawPull declares a raw PULL resource polled every interval.
func rawPull(name string, interval time.Duration) opts.ResourceOption {
	return Raw(opts.ResourceDefinition{Name: name}, opts.FetchDefinition{Fetch: opts.PULL, PullInterval: interval})
}

func serverConn(fb *fakeBackend, token string) *opts.ConnectionOption {
	return &opts.ConnectionOption{Addr: fb.URL(), Namespace: "n", App: "a", Token: token, Env: testEnvName}
}

// startConsumer starts c and closes it when the test ends.
func startConsumer(t *testing.T, c *Consumer) {
	t.Helper()
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
}

// tick waits for the consumer to arm a timer of duration d, then fires it.
// Because the consumer arms the next timer only after finishing the work for
// this one, the following tick cannot pass until that work is done.
func tick(t *testing.T, te *testEnv, d time.Duration) {
	t.Helper()
	te.clock.awaitWaiter(t, d)
	te.clock.fire(d)
}

func getRaw(t *testing.T, c *Consumer, name string) []byte {
	t.Helper()
	v, err := Get[[]byte](c, name)
	if err != nil {
		t.Fatalf("Get %q: %v", name, err)
	}
	return v
}

// ---- PULL ----

// C-19: a 200 body is stored byte for byte.
func TestC19_PullStoresBodyUnchanged(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	body := []byte(`{"port": 8080,  "log_level":"info"}`)
	fb.set("raw", fakeResource{body: body, version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)
	if got := getRaw(t, c, "raw"); string(got) != string(body) {
		t.Fatalf("body = %q", got)
	}
}

// C-20: a 200 without a version header is accepted, with version unknown.
func TestC20_MissingVersionIsNotAnError(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("x")})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)
	if _, version, _ := c.byName["raw"].snapshot(); version != "" {
		t.Fatalf("version = %q, want unknown", version)
	}
}

// C-21: a resource that was never deployed and has no fallback fails Start.
func TestC21_NeverDeployedFailsStart(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	err := c.Start(context.Background())
	if !errors.Is(err, ErrResourceNotLoaded) {
		t.Fatalf("Start = %v, want RESOURCE_NOT_LOADED", err)
	}
	_ = c.Close()
}

// C-22: the token is sent as x-token, and a 401 fails Start.
func TestC22_TokenSentAndUnauthorizedFails(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{status: http.StatusUnauthorized})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "app-token")})
	if err := c.Start(context.Background()); !errors.Is(err, ErrResourceNotLoaded) {
		t.Fatalf("Start = %v", err)
	}
	reqs := fb.requests()
	if len(reqs) == 0 || reqs[0].token != "app-token" {
		t.Fatalf("requests = %+v, want x-token app-token", reqs)
	}
	_ = c.Close()
}

// C-23: a 304 for the known version keeps the value and changes nothing.
func TestC23_NotModifiedKeepsValue(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("v1"), etag: `"1"`})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "t")})
	startConsumer(t, c)

	tick(t, te, 10*time.Second)
	te.clock.awaitWaiter(t, 10*time.Second)

	reqs := fb.requests()
	last := reqs[len(reqs)-1]
	if last.ifNoneMatch != `"1"` {
		t.Fatalf("poll sent If-None-Match %q, want \"1\"", last.ifNoneMatch)
	}
	if got := getRaw(t, c, "raw"); string(got) != "v1" {
		t.Fatalf("value = %q", got)
	}
	if te.log.count("resource.changed") != 0 {
		t.Fatal("a 304 produced a change")
	}
}

// C-24: a backend that ignores the conditional header and returns the same digest is no change.
func TestC24_SameDigestIsNoChangeAndNoReport(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("v1"), etag: `"1"`, ignoreConditional: true})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "t")})
	startConsumer(t, c)
	waitForReports(t, fb, 1)
	reportsAfterLoad := len(fb.reportList())

	tick(t, te, 10*time.Second)
	te.clock.awaitWaiter(t, 10*time.Second)

	if te.log.count("resource.changed") != 0 {
		t.Fatal("same digest was reported as a change")
	}
	if len(fb.reportList()) != reportsAfterLoad {
		t.Fatal("same digest triggered a consumer report")
	}
}

// C-38: a long outage produces one resource.stale, and the value is kept.
func TestC38_OutageLogsOnceAndKeepsValue(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("good"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)

	fb.set("raw", fakeResource{status: http.StatusInternalServerError})
	for i := 0; i < 30; i++ {
		tick(t, te, 10*time.Second)
	}
	te.clock.awaitWaiter(t, 10*time.Second)

	if n := te.log.count("resource.stale"); n != 1 {
		t.Fatalf("resource.stale logged %d times, want 1", n)
	}
	if got := getRaw(t, c, "raw"); string(got) != "good" {
		t.Fatalf("value after outage = %q", got)
	}
}

// C-39: recovery is logged once, and the new value is picked up.
func TestC39_RecoveryLogsOnce(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("v1"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)

	fb.set("raw", fakeResource{status: http.StatusBadGateway})
	tick(t, te, 10*time.Second)
	te.clock.awaitWaiter(t, 10*time.Second)
	fb.set("raw", fakeResource{body: []byte("v2"), version: "2"})
	tick(t, te, 10*time.Second)
	te.clock.awaitWaiter(t, 10*time.Second)

	if te.log.count("resource.recovered") != 1 {
		t.Fatalf("resource.recovered logged %d times", te.log.count("resource.recovered"))
	}
	if got := getRaw(t, c, "raw"); string(got) != "v2" {
		t.Fatalf("value = %q, want v2", got)
	}
}

// C-40: a 404 after success is a failure and does not clear the value.
func TestC40_NotFoundAfterSuccessKeepsValue(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("kept"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)

	fb.set("raw", fakeResource{status: http.StatusNotFound})
	tick(t, te, 10*time.Second)
	te.clock.awaitWaiter(t, 10*time.Second)

	if got := getRaw(t, c, "raw"); string(got) != "kept" {
		t.Fatalf("value = %q", got)
	}
	if te.log.count("resource.stale") != 1 {
		t.Fatal("404 after success was not logged as stale")
	}
}

// C-41: Once fetches once and never polls.
func TestC41_OnceFetchesOnce(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("x"), version: "1"})
	res := Raw(opts.ResourceDefinition{Name: "raw"}, opts.FetchDefinition{Fetch: opts.PULL, Once: true, PullInterval: 10 * time.Second})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{res}, Connection: serverConn(fb, "")})
	startConsumer(t, c)
	if n := len(fb.requests()); n != 1 {
		t.Fatalf("requests = %d, want 1", n)
	}
	if te.clock.hasWaiter(10 * time.Second) {
		t.Fatal("Once resource armed a poll timer")
	}
}

// ---- VOLUME ----

func volumeRes(name, path string) opts.ResourceOption {
	return Raw(opts.ResourceDefinition{Name: name, Path: path}, opts.FetchDefinition{Fetch: opts.VOLUME})
}

// C-34: the value comes from {Path}/_{Name}.
func TestC34_VolumeReadsUnderscoredFile(t *testing.T) {
	te := newTestEnv(t)
	te.fs.set("/etc/sailor/_config", []byte("mounted"))
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{volumeRes("config", "/etc/sailor")}, Connection: serverConn(newFakeBackend(t), "")})
	startConsumer(t, c)
	if got := getRaw(t, c, "config"); string(got) != "mounted" {
		t.Fatalf("value = %q", got)
	}
}

// C-35: a missing file with a fallback uses the fallback and says so.
func TestC35_VolumeMissingUsesFallback(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.setFile("/fallback/a-config.sailor.fall", fakeResource{body: []byte("from-fallback")})
	te.env["SAILOR_FALLBACK_BASE_URL"] = fb.URL() + "/fallback/"
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{volumeRes("config", "/etc/sailor")}, Connection: serverConn(fb, "")})
	startConsumer(t, c)
	if got := getRaw(t, c, "config"); string(got) != "from-fallback" {
		t.Fatalf("value = %q", got)
	}
	if te.log.count("resource.fallback_used") != 1 {
		t.Fatal("fallback use was not logged")
	}
}

// C-36: a symlink-style event re-reads the file after the debounce and reports once.
func TestC36_VolumeChangeAfterDebounce(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	te.fs.set("/etc/sailor/_config", []byte("v1"))
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{volumeRes("config", "/etc/sailor")}, Connection: serverConn(fb, "t")})
	startConsumer(t, c)
	reportsBefore := len(fb.reportList())

	te.fs.set("/etc/sailor/_config", []byte("v2"))
	te.watcher.events <- watchEvent{Path: "/etc/sailor/..data"}
	tick(t, te, watchDebounce)
	te.log.await(t, "resource.changed")

	if got := getRaw(t, c, "config"); string(got) != "v2" {
		t.Fatalf("value = %q, want v2", got)
	}
	waitForReports(t, fb, reportsBefore+1)
}

// C-37: a source that gives no version does not clear a known version (CONS-STATE-3).
func TestC37_UnknownVersionKeepsKnownVersion(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	r := c.byName["raw"]
	now := te.clock.Now()
	if _, err := c.storeSource(r, []byte("a"), "7", now); err != nil {
		t.Fatal(err)
	}
	if _, err := c.storeSource(r, []byte("b"), "", now); err != nil {
		t.Fatal(err)
	}
	if _, version, _ := r.snapshot(); version != "7" {
		t.Fatalf("version = %q, want 7", version)
	}
}

// ---- DEV ----

func devRes(name string, encrypted bool) opts.ResourceOption {
	return Raw(opts.ResourceDefinition{Name: name, Encrypted: encrypted}, opts.FetchDefinition{Fetch: opts.DEV})
}

func devCachePathFor(name string) string {
	return filepath.Join("/home/test", ".sailor", "cache", "n-a-"+testEnvName+"-"+name+".json")
}

// C-44: cold cache writes the file and its sidecar.
func TestC44_DevColdCacheWritesFileAndSidecar(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("config", fakeResource{body: []byte(`{"a":1}`), version: "3"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{devRes("config", false)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)

	if got, ok := te.fs.get(devCachePathFor("config")); !ok || string(got) != `{"a":1}` {
		t.Fatalf("cache file = %q, ok=%v", got, ok)
	}
	if _, ok := te.fs.get(devCachePathFor("config") + ".meta.json"); !ok {
		t.Fatal("sidecar not written")
	}
}

func seedPristineCache(te *testEnv, name string, body []byte, version string) {
	te.fs.set(devCachePathFor(name), body)
	meta := `{"version":"` + version + `","hash":"` + hexSHA256(body) + `"}`
	te.fs.set(devCachePathFor(name)+".meta.json", []byte(meta))
}

// C-45: a pristine cache is overwritten when the server has a new digest.
func TestC45_DevPristineOverwrittenOnNewDigest(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	seedPristineCache(te, "config", []byte("old"), "1")
	fb.set("config", fakeResource{body: []byte("new"), version: "2"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{devRes("config", false)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)
	if got, _ := te.fs.get(devCachePathFor("config")); string(got) != "new" {
		t.Fatalf("cache = %q, want new", got)
	}
}

// C-46: a pristine cache is used when the server cannot be reached, with a stale log.
func TestC46_DevPristineUsedWhenServerDown(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	seedPristineCache(te, "config", []byte("cached"), "1")
	fb.set("config", fakeResource{status: http.StatusInternalServerError})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{devRes("config", false)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)
	if got := getRaw(t, c, "config"); string(got) != "cached" {
		t.Fatalf("value = %q", got)
	}
	if te.log.count("resource.stale") != 1 {
		t.Fatal("server outage was not logged as stale")
	}
}

// C-47: a hand-edited cache is kept, not overwritten, and the server is not asked.
func TestC47_DevLocalEditNeverOverwritten(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	seedPristineCache(te, "config", []byte("original"), "1")
	te.fs.set(devCachePathFor("config"), []byte("my local edit"))
	fb.set("config", fakeResource{body: []byte("upstream"), version: "2"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{devRes("config", false)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)

	if got, _ := te.fs.get(devCachePathFor("config")); string(got) != "my local edit" {
		t.Fatalf("local edit overwritten: %q", got)
	}
	if got := getRaw(t, c, "config"); string(got) != "my local edit" {
		t.Fatalf("value = %q", got)
	}
	if te.log.count("resource.local_override") != 1 {
		t.Fatal("local override not logged")
	}
	for _, r := range fb.requests() {
		if r.path == "/api/v1/resource/n/a/config" {
			t.Fatalf("server was asked for a locally edited resource: %+v", r)
		}
	}
}

// C-48: a cache with no sidecar counts as locally edited.
func TestC48_DevMissingSidecarIsLocalEdit(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	te.fs.set(devCachePathFor("config"), []byte("no sidecar"))
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{devRes("config", false)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)
	if te.log.count("resource.local_override") != 1 {
		t.Fatal("missing sidecar not treated as a local edit")
	}
}

// C-49: DEV never polls after start.
func TestC49_DevDoesNotPoll(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	seedPristineCache(te, "config", []byte("c"), "1")
	fb.set("config", fakeResource{body: []byte("c"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{devRes("config", false)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)
	before := len(fb.requests())
	if te.clock.hasWaiter(10 * time.Second) {
		t.Fatal("DEV armed a poll timer")
	}
	if len(fb.requests()) != before {
		t.Fatal("DEV issued requests after start")
	}
}

// ---- fallback ----

// C-42: a fallback that answers non-200 is an error and its body is not stored.
func TestC42_FallbackNon200IsNotStored(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.setFile("/fallback/a-config.sailor.fall", fakeResource{status: http.StatusNotFound, body: []byte("not found page")})
	te.env["SAILOR_FALLBACK_BASE_URL"] = fb.URL() + "/fallback"
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{volumeRes("config", "/etc/sailor")}, Connection: serverConn(fb, "")})
	err := c.Start(context.Background())
	if !errors.Is(err, ErrResourceNotLoaded) {
		t.Fatalf("Start = %v, want RESOURCE_NOT_LOADED", err)
	}
	if _, getErr := Get[[]byte](c, "config"); getErr == nil {
		t.Fatal("404 body was stored as content")
	}
	_ = c.Close()
}

// C-43: with no fallback base URL configured, fallback fails at once and no request is made.
func TestC43_NoFallbackBaseFailsImmediately(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{volumeRes("config", "/etc/sailor")}, Connection: serverConn(fb, "")})
	err := c.Start(context.Background())
	if !errors.Is(err, ErrResourceNotLoaded) {
		t.Fatalf("Start = %v", err)
	}
	if len(fb.requests()) != 0 {
		t.Fatalf("requests made: %+v", fb.requests())
	}
	_ = c.Close()
}

// waitForReports blocks until the backend has received n consumer reports.
func waitForReports(t *testing.T, fb *fakeBackend, n int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for len(fb.reportList()) < n {
		select {
		case <-fb.reportSignal:
		case <-deadline:
			t.Fatalf("got %d reports, want %d", len(fb.reportList()), n)
		}
	}
}
