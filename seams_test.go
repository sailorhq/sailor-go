package sailor

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

// SAILOR_CONSUMER.md section 4.1: each standard constructor produces exactly these values.
func TestStandardConstructorsMatchSpecTable(t *testing.T) {
	cases := []struct {
		name      string
		opt       opts.ResourceOption
		wantName  string
		wantFetch opts.FetchOption
		encrypted bool
		path      string
	}{
		{"ConfigMapDefault", ConfigMapDefault[map[string]any](), "config", opts.VOLUME, false, CONFIG_MOUNT_PATH},
		{"SecretsDefault", SecretsDefault[map[string]any](), "secret", opts.VOLUME, true, SECRET_MOUNT_PATH},
		{"ConfigPullDefault", ConfigPullDefault[map[string]any](), "config", opts.PULL, false, ""},
		{"SecretsPullDefault", SecretsPullDefault[map[string]any](), "secret", opts.PULL, true, ""},
		{"ConfigDevDefault", ConfigDevDefault[map[string]any](), "config", opts.DEV, false, ""},
		{"SecretsDevDefault", SecretsDevDefault[map[string]any](), "secret", opts.DEV, true, ""},
		{"MiscPullDefault", MiscPullDefault("extra"), "extra", opts.PULL, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.opt.Def.Name != tc.wantName || tc.opt.FetchDef.Fetch != tc.wantFetch ||
				tc.opt.Def.Encrypted != tc.encrypted || tc.opt.Def.Path != tc.path {
				t.Fatalf("got %+v / %+v", tc.opt.Def, tc.opt.FetchDef)
			}
			if tc.opt.FetchDef.Fetch == opts.PULL && tc.opt.FetchDef.PullInterval != 10*time.Second {
				t.Fatalf("PULL interval = %s, want 10s", tc.opt.FetchDef.PullInterval)
			}
		})
	}
}

func TestRawStoresBytesWithoutDecoder(t *testing.T) {
	r := MiscPullDefault("bytes")
	if r.Decode != nil {
		t.Fatal("Raw resource has a decoder")
	}
	typed := ConfigPullDefault[map[string]any]()
	if typed.Decode == nil {
		t.Fatal("typed resource has no decoder")
	}
}

func TestErrorUnwrapKeepsCause(t *testing.T) {
	cause := errors.New("root cause")
	err := wrapf(CodeDecodeFailed, cause, "context")
	if !errors.Is(err, cause) {
		t.Fatal("wrapped cause is not reachable with errors.Is")
	}
	if !errors.Is(err, &Error{Code: CodeDecodeFailed}) {
		t.Fatal("code is not matchable")
	}
}

// NewConsumer uses the real environment, clock and filesystem. It must
// construct without starting anything.
func TestPublicNewConsumerConstructs(t *testing.T) {
	c, err := NewConsumer(opts.InitOption{
		Resources:  []opts.ResourceOption{ConfigPullDefault[map[string]any]()},
		Connection: &opts.ConnectionOption{Addr: "http://127.0.0.1:1", Namespace: "n", App: "a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRealFilesystemRoundTrip(t *testing.T) {
	fs := realFS{}
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b")
	if err := fs.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(nested, "f.txt")
	if err := fs.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := fs.ReadFile(path)
	if err != nil || string(got) != "hello" {
		t.Fatalf("read = %q, %v", got, err)
	}
	if _, err := fs.ReadFile(filepath.Join(dir, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file error = %v", err)
	}
}

func TestRealClockAdvances(t *testing.T) {
	clk := realClock{}
	before := clk.Now()
	select {
	case <-clk.After(time.Nanosecond):
	case <-time.After(5 * time.Second):
		t.Fatal("After never fired")
	}
	if !clk.Now().After(before) && !clk.Now().Equal(before) {
		t.Fatal("clock went backwards")
	}
}

// The fsnotify adapter forwards a real file creation as a watch event.
func TestFsnotifyWatcherForwardsCreation(t *testing.T) {
	dir := t.TempDir()
	w, err := newFsnotifyWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Add(dir); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "_config")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-w.Events():
		if ev.Path == "" {
			t.Fatal("event has no path")
		}
	case err := <-w.Errors():
		t.Fatalf("watcher error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("no event for a file creation")
	}
}

func TestDefaultDepsAreComplete(t *testing.T) {
	d := defaultDeps()
	if d.clock == nil || d.fs == nil || d.getenv == nil || d.homeDir == nil || d.hostname == nil || d.newWatcher == nil {
		t.Fatal("defaultDeps leaves a seam unset")
	}
	if _, err := d.hostname(); err != nil {
		t.Fatalf("hostname: %v", err)
	}
}
