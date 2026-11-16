package sailor

import (
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

// C-50: the first load reports once, with digest sha256:<hex of bytes>.
func TestC50_FirstLoadReportsOnce(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	body := []byte("first")
	fb.set("raw", fakeResource{body: body, version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "t")})
	startConsumer(t, c)
	waitForReports(t, fb, 1)

	reps := fb.reportList()
	if len(reps) != 1 {
		t.Fatalf("reports = %d, want 1", len(reps))
	}
	if reps[0].Digest != "sha256:"+hex.EncodeToString(sha256Sum(body)) {
		t.Fatalf("digest = %q", reps[0].Digest)
	}
	if reps[0].Version != "1" {
		t.Fatalf("version = %q, want 1", reps[0].Version)
	}
}

// C-51: a changed digest logs resource.changed and reports once.
func TestC51_ChangeLogsAndReports(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("v1"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "t")})
	startConsumer(t, c)
	waitForReports(t, fb, 1)

	fb.set("raw", fakeResource{body: []byte("v2"), version: "2"})
	tick(t, te, 10*time.Second)
	te.log.await(t, "resource.changed")
	te.clock.awaitWaiter(t, 10*time.Second)
	waitForReports(t, fb, 2)
	if got := fb.reportList()[1].Version; got != "2" {
		t.Fatalf("second report version = %q", got)
	}
}

// C-52: an unchanged digest sends no report.
func TestC52_UnchangedDigestNoReport(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("same"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "t")})
	startConsumer(t, c)
	waitForReports(t, fb, 1)
	tick(t, te, 10*time.Second)
	te.clock.awaitWaiter(t, 10*time.Second)
	if n := len(fb.reportList()); n != 1 {
		t.Fatalf("reports = %d, want 1", n)
	}
}

// C-53: the heartbeat re-reports the current state on its interval.
func TestC53_HeartbeatReportsCurrentState(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("steady"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "t")})
	startConsumer(t, c)
	waitForReports(t, fb, 1)

	tick(t, te, heartbeatInterval)
	waitForReports(t, fb, 2)
	if fb.reportList()[0].Digest != fb.reportList()[1].Digest {
		t.Fatal("heartbeat digest differs from the stored digest")
	}
}

// C-54: the report digest is sha256: plus 64 lowercase hex characters.
func TestC54_ReportDigestFormat(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("x"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "t")})
	startConsumer(t, c)
	waitForReports(t, fb, 1)
	d := fb.reportList()[0].Digest
	if !strings.HasPrefix(d, "sha256:") || len(d) != len("sha256:")+64 || d != strings.ToLower(d) {
		t.Fatalf("digest %q is not sha256: plus 64 lowercase hex", d)
	}
}

// C-55: a non-2xx report is logged with its status and the body.
func TestC55_RejectedReportIsLogged(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.reportStatus = http.StatusInternalServerError
	fb.set("raw", fakeResource{body: []byte("x"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "t")})
	startConsumer(t, c)
	ev := te.log.await(t, "report.failed")
	if ev.kv["status"] != http.StatusInternalServerError {
		t.Fatalf("status = %v", ev.kv["status"])
	}
	if !strings.Contains(ev.kv["message"].(string), "report rejected") {
		t.Fatalf("message = %v", ev.kv["message"])
	}
}

// C-56: with no token nothing is sent, and the skip is logged once per resource.
func TestC56_NoTokenNoReportLoggedOnce(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("x"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "")})
	startConsumer(t, c)
	if n := te.log.count("report.skipped_no_token"); n != 1 {
		t.Fatalf("skip logged %d times, want 1", n)
	}
	if n := len(fb.reportList()); n != 0 {
		t.Fatalf("reports sent without a token: %d", n)
	}
}

// C-57: pod details are included only when set.
func TestC57_PodDetailsOmittedWhenUnset(t *testing.T) {
	te := newTestEnv(t)
	te.env["POD_IP"] = "10.0.0.5"
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("x"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "t")})
	startConsumer(t, c)
	waitForReports(t, fb, 1)
	d := fb.reportList()[0].Details
	if d["pod_ip"] != "10.0.0.5" {
		t.Fatalf("details = %v", d)
	}
	if _, ok := d["pod_name"]; ok {
		t.Fatalf("pod_name present with no value: %v", d)
	}
}

// C-58: an unreadable host name becomes "unknown".
func TestC58_UnknownIdentity(t *testing.T) {
	te := newTestEnv(t)
	te.deps.hostname = func() (string, error) { return "", errors.New("no host") }
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("x"), version: "1"})
	c := te.newConsumer(t, opts.InitOption{Resources: []opts.ResourceOption{rawPull("raw", 10*time.Second)}, Connection: serverConn(fb, "t")})
	startConsumer(t, c)
	waitForReports(t, fb, 1)
	if got := fb.reportList()[0].Consumer; got != "unknown" {
		t.Fatalf("consumer = %q, want unknown", got)
	}
}

// blockingPOST makes every POST hang until its context ends.
type blockingPOST struct{}

func (blockingPOST) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodPost {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}
	return http.DefaultTransport.RoundTrip(req)
}

// C-59: a report stuck in the network does not block Get.
func TestC59_BlockedReportDoesNotBlockGet(t *testing.T) {
	te := newTestEnv(t)
	fb := newFakeBackend(t)
	fb.set("raw", fakeResource{body: []byte("ready"), version: "1"})
	in := opts.InitOption{
		Resources:     []opts.ResourceOption{rawPull("raw", 10*time.Second)},
		Connection:    serverConn(fb, "t"),
		ReportTimeout: time.Millisecond,
		HTTPClient:    &http.Client{Transport: blockingPOST{}},
	}
	c := te.newConsumer(t, in)
	startConsumer(t, c)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if got := getRaw(t, c, "raw"); string(got) != "ready" {
			t.Errorf("value = %q", got)
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Get blocked behind a report")
	}
}
