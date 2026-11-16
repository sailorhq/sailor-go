package sailor

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

func oneResource() []opts.ResourceOption {
	return []opts.ResourceOption{ConfigPullDefault[map[string]any]()}
}

// C-1
func TestC1_EmptyResourcesFails(t *testing.T) {
	te := newTestEnv(t)
	_, err := newConsumer(opts.InitOption{Connection: &opts.ConnectionOption{Addr: "h", Namespace: "n", App: "a"}}, te.deps)
	if !errors.Is(err, ErrEmptyResources) {
		t.Fatalf("got %v, want EMPTY_RESOURCES", err)
	}
}

// C-2
func TestC2_EnvURIReplacesConnection(t *testing.T) {
	te := newTestEnv(t)
	te.env["SAILOR_URI"] = "sailor://ak:sk@uri-host:7766/uns/uapp"
	c := te.newConsumer(t, opts.InitOption{
		Resources:  oneResource(),
		Connection: &opts.ConnectionOption{Addr: "http://code-host", Namespace: "cns", App: "capp"},
	})
	if c.conn.namespace != "uns" || c.conn.app != "uapp" || c.conn.addr != "http://uri-host:7766" {
		t.Fatalf("connection = %+v, want URI values", c.conn)
	}
}

// C-3
func TestC3_LocalConfigNeedsNamespaceAndApp(t *testing.T) {
	te := newTestEnv(t)
	_, err := newConsumer(opts.InitOption{Resources: oneResource(), UseLocalConfig: true}, te.deps)
	if !errors.Is(err, &Error{Code: CodeMissingNamespaceApp}) {
		t.Fatalf("got %v, want MISSING_NAMESPACE_APP", err)
	}
}

// C-4
func TestC4_NoConnectionFails(t *testing.T) {
	te := newTestEnv(t)
	_, err := newConsumer(opts.InitOption{Resources: oneResource()}, te.deps)
	if !errors.Is(err, ErrNoConnection) {
		t.Fatalf("got %v, want NO_CONNECTION", err)
	}
}

// C-5
func TestC5_SailorTokenIgnoredForFieldConnection(t *testing.T) {
	te := newTestEnv(t)
	te.env["SAILOR_TOKEN"] = "from-env"
	c := te.newConsumer(t, opts.InitOption{
		Resources:  oneResource(),
		Connection: &opts.ConnectionOption{Addr: "h", Namespace: "n", App: "a", Token: "field"},
	})
	if c.conn.token != "field" {
		t.Fatalf("token = %q, want the field value", c.conn.token)
	}
}

// C-6
func TestC6_URIUserinfoIsPercentDecoded(t *testing.T) {
	c, err := parseURI("sailor://a%40b:c%3Ad@host.example:7766/ns/app?scheme=https")
	if err != nil {
		t.Fatal(err)
	}
	if c.accessKey != "a@b" || c.secretKey != "c:d" || c.addr != "https://host.example:7766" || c.namespace != "ns" || c.app != "app" {
		t.Fatalf("parsed = %+v", c)
	}
}

// C-7
func TestC7_URIDefaultsToHTTP(t *testing.T) {
	c, err := parseURI("sailor://a:b@host/ns/app")
	if err != nil {
		t.Fatal(err)
	}
	if c.addr != "http://host" {
		t.Fatalf("addr = %q, want http://host", c.addr)
	}
}

// C-8 to C-13: URI grammar errors.
func TestC8ToC13_URIErrors(t *testing.T) {
	cases := []struct {
		name string
		uri  string
		want Code
	}{
		{"C-8 bad scheme parameter", "sailor://a:b@host/ns/app?scheme=ftp", CodeInvalidScheme},
		{"C-9 wrong URI scheme", "http://a:b@host/ns/app", CodeInvalidURIPrefix},
		{"C-10 no access key", "sailor://:b@host/ns/app", CodeMissingAccessKey},
		{"C-11 no secret key", "sailor://a@host/ns/app", CodeMissingSecretKey},
		{"C-12 missing app", "sailor://a:b@host/ns", CodeMissingPathComponents},
		{"C-13 extra path component", "sailor://a:b@host/ns/app/extra", CodeMissingPathComponents},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseURI(tc.uri)
			var se *Error
			if !errors.As(err, &se) || se.Code != tc.want {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
		})
	}
}

// C-14 to C-16: address normalisation.
func TestC14ToC16_AddrNormalisation(t *testing.T) {
	if got, _ := normalizeAddr("host:7766"); got != "http://host:7766" {
		t.Fatalf("C-14 got %q", got)
	}
	if got, _ := normalizeAddr("https://host"); got != "https://host" {
		t.Fatalf("C-15 got %q", got)
	}
	if _, err := normalizeAddr(""); !errors.Is(err, ErrNoConnection) {
		t.Fatalf("C-16 got %v", err)
	}
}

// C-17 and C-18: insecure transport warning, loopback exempt.
func TestC17And18_InsecureTransportWarning(t *testing.T) {
	cases := []struct {
		name     string
		addr     string
		wantWarn bool
	}{
		{"C-17 non-loopback over http", "http://example.internal", true},
		{"C-18 loopback over http", "http://localhost:7766", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			te := newTestEnv(t)
			te.newConsumer(t, opts.InitOption{
				Resources:  oneResource(),
				Connection: &opts.ConnectionOption{Addr: tc.addr, Namespace: "n", App: "a", Token: "t"},
			})
			got := te.log.count("config.insecure_transport") > 0
			if got != tc.wantWarn {
				t.Fatalf("insecure warning = %v, want %v", got, tc.wantWarn)
			}
		})
	}
}

// Local developer config (section 12): the active environment's host and token.
func TestLocalConfigResolvesActiveEnvironment(t *testing.T) {
	te := newTestEnv(t)
	te.fs.set(filepath.Join("/home/test", ".sailor", "config"), []byte(`{
		"env": "sit",
		"manifest": {"envs": [{"name": "sit", "host": "sit.example:7766"}]},
		"tokens": {"sit": "local-token"},
		"user": "someone"
	}`))
	c := te.newConsumer(t, opts.InitOption{
		Resources:      oneResource(),
		UseLocalConfig: true,
		Connection:     &opts.ConnectionOption{Namespace: "n", App: "a"},
	})
	if c.conn.addr != "http://sit.example:7766" || c.conn.token != "local-token" || c.conn.env != "sit" {
		t.Fatalf("connection = %+v", c.conn)
	}
}

func TestLocalConfigMissingFileFails(t *testing.T) {
	te := newTestEnv(t)
	_, err := newConsumer(opts.InitOption{
		Resources:      oneResource(),
		UseLocalConfig: true,
		Connection:     &opts.ConnectionOption{Namespace: "n", App: "a"},
	}, te.deps)
	if !errors.Is(err, &Error{Code: CodeLocalConfigNotFound}) {
		t.Fatalf("got %v, want LOCAL_CONFIG_NOT_FOUND", err)
	}
}

func TestDefaultDepsUsesRealEnvironment(t *testing.T) {
	d := defaultDeps()
	if d.getenv == nil || d.fs == nil || d.clock == nil {
		t.Fatal("defaultDeps left a seam unset")
	}
	_ = os.Getenv
}
