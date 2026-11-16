package sailor

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

// C-32: a server header that disagrees with the declaration is logged, and the declaration decides.
func TestC32_EncryptedHeaderMismatchIsLoggedAndDeclarationWins(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	body := testEncryptedBody(t, "ak", "sk", map[string]string{"k": "v"})
	fb.set("secret", fakeResource{body: body, version: "1", encrypted: false})
	res := Resource[map[string]string](opts.ResourceDefinition{Name: "secret", Encrypted: true}, opts.FetchDefinition{Fetch: opts.PULL, PullInterval: 10 * time.Second})
	in := opts.InitOption{
		Resources:  []opts.ResourceOption{res},
		Connection: &opts.ConnectionOption{Addr: fb.URL(), Namespace: "n", App: "a", AccessKey: "ak", SecretKey: "sk"},
	}
	c := te.newConsumer(t, in)
	startConsumer(t, c)

	ev := te.log.await(t, "resource.encryption_mismatch")
	if ev.kv["declared"] != true || ev.kv["server"] != "false" {
		t.Fatalf("mismatch event = %v", ev.kv)
	}
	got, err := Get[map[string]string](c, "secret")
	if err != nil || got["k"] != "v" {
		t.Fatalf("value = %v, err = %v; declaration should have decrypted it", got, err)
	}
}

// C-60: two consumers in one process keep independent state.
func TestC60_ConsumersAreIndependent(t *testing.T) {
	te := newTestEnv(t)
	fbA := newFakeBackend(t)
	fbB := newFakeBackend(t)
	fbA.set("raw", fakeResource{body: []byte("A"), version: "1"})
	fbB.set("raw", fakeResource{body: []byte("B"), version: "1"})
	a := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fbA, "")})
	b := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fbB, "")})
	startConsumer(t, a)
	startConsumer(t, b)
	if getRaw(t, a, "raw")[0] != 'A' || getRaw(t, b, "raw")[0] != 'B' {
		t.Fatal("consumers share state")
	}
}

// C-61: Close stops a consumer that is polling, and returns.
func TestC61_CloseStopsPolling(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("x"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	te.clock.awaitWaiter(t, 10*time.Second)
	done := make(chan error, 1)
	go func() { done <- c.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return")
	}
}

// C-62: Close is idempotent.
func TestC62_CloseTwiceIsSafe(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("x"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

// C-63: Get after Close returns the last value.
func TestC63_GetAfterCloseReturnsLastValue(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("last"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)
	_ = c.Close()
	if got := getRaw(t, c, "raw"); string(got) != "last" {
		t.Fatalf("value after close = %q", got)
	}
}

// C-65: no secret appears in any log line, including on error paths.
func TestC65_SecretsNeverLogged(t *testing.T) {
	const (
		accessKey = "ACCESS-KEY-SECRET-1234"
		secretKey = "SECRET-KEY-SECRET-5678"
		token     = "TOKEN-SECRET-9999"
	)
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("secret", fakeResource{body: []byte(`{"bad":`), version: "1", encrypted: true})
	res := Resource[map[string]string](opts.ResourceDefinition{Name: "secret", Encrypted: true}, opts.FetchDefinition{Fetch: opts.PULL, PullInterval: 10 * time.Second})
	c := te.newConsumer(t, opts.InitOption{
		Resources:  []opts.ResourceOption{res},
		Connection: &opts.ConnectionOption{Addr: fb.URL(), Namespace: "n", App: "a", AccessKey: accessKey, SecretKey: secretKey, Token: token},
	})
	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected a decode failure")
	}
	_ = c.Close()
	for _, e := range te.log.events {
		for k, v := range e.kv {
			s := strings.TrimSpace(strings.ReplaceAll(toString(v), "\n", " "))
			for _, secret := range []string{accessKey, secretKey, token} {
				if strings.Contains(s, secret) || strings.Contains(k, secret) {
					t.Fatalf("secret leaked in %s.%s", e.name, k)
				}
			}
		}
	}
	if strings.Contains(err.Error(), accessKey) || strings.Contains(err.Error(), secretKey) || strings.Contains(err.Error(), token) {
		t.Fatalf("secret leaked in error: %v", err)
	}
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// C-66: the URI userinfo is not logged.
func TestC66_URIUserinfoRedactedInLogs(t *testing.T) {
	te := newTestEnv(t)
	te.env["SAILOR_URI"] = "sailor://UNIQUE-AK-42:UNIQUE-SK-43@backend.example:7766/n/a"
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}})
	_ = c
	ev := te.log.await(t, "config.resolved")
	for _, v := range ev.kv {
		s := toString(v)
		if strings.Contains(s, "UNIQUE-AK-42") || strings.Contains(s, "UNIQUE-SK-43") {
			t.Fatalf("userinfo logged: %s", s)
		}
	}
	if ev.kv["addr"] != "http://backend.example:7766" {
		t.Fatalf("addr = %v", ev.kv["addr"])
	}
}

// C-68: critical events carry their required fields.
func TestC68_CriticalEventsCarryFields(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("x"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)

	want := map[string][]string{
		"config.resolved":         {"source", "addr", "namespace", "app", "token"},
		"resource.loaded":         {"resource", "mode", "version", "digest"},
		"report.skipped_no_token": {"resource"},
		"consumer.started":        {"resources"},
	}
	for name, fields := range want {
		ev := te.log.await(t, name)
		for _, f := range fields {
			if _, ok := ev.kv[f]; !ok {
				t.Fatalf("%s is missing field %q", name, f)
			}
		}
	}
}

// C-69: Get with the wrong type returns TYPE_MISMATCH.
func TestC69_WrongTypeIsAnError(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("x"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)
	if _, err := Get[int](c, "raw"); err == nil || !strings.Contains(err.Error(), string(CodeTypeMismatch)) {
		t.Fatalf("got %v, want TYPE_MISMATCH", err)
	}
}

// Plan verification 6: a full lifecycle leaves no goroutines behind. Keep-alives
// are disabled so pooled connections cannot account for the difference.
func TestLifecycleLeavesNoGoroutines(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("x"), version: "1"})
	fb.set("cfg", fakeResource{body: []byte(`{}`), version: "1"})
	transport := &http.Transport{DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)

	baseline := runtime.NumGoroutine()
	c := te.newConsumer(t, opts.InitOption{
		Resources: []opts.ResourceOption{
			rawPull("raw", 10*time.Second),
			typedPull("cfg"),
		},
		Connection: serverConn(fb, "token"),
		HTTPClient: &http.Client{Transport: transport},
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	te.clock.awaitWaiter(t, 10*time.Second)
	te.clock.fire(10 * time.Second)
	te.clock.awaitWaiter(t, 10*time.Second)
	waitForReports(t, fb, 2)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	transport.CloseIdleConnections()

	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > baseline && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if n := runtime.NumGoroutine(); n > baseline {
		buf := make([]byte, 1<<16)
		t.Fatalf("goroutines: %d after Close, baseline %d\n%s", n, baseline, buf[:runtime.Stack(buf, true)])
	}
}
