package sailor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

func TestFallbackConfig(t *testing.T) {
	type DummyConfig struct {
		App string `json:"app"`
	}

	app := "sailor-sailing-the-containers"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test-config.sailor.fall" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(DummyConfig{App: app})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	os.Setenv(ENV_SAILOR_FALLBACK_BASE_URL, server.URL)
	defer os.Unsetenv(ENV_SAILOR_FALLBACK_BASE_URL)

	consumer, err := NewConsumer[DummyConfig, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
					Path: "/invalid/path/for/volume",
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
				FallbackEnabled: true,
			},
		},
		Connection: &opts.ConnectionOption{
			Addr:          "http://localhost:7766", // Invalid/unreachable to force failure if pull
			Namespace:     "test",
			App:           "test",
			AccessKey:     "ak",
			SecretKey:     "sk",
			SocketTimeout: time.Second * 5,
		},
	})

	if err != nil {
		t.Error(err)
		return
	}

	if err = consumer.Start(); err != nil {
		t.Error(err)
		return
	}

	config, err := consumer.Get()
	if err != nil {
		t.Error(err)
		return
	}

	if config.App != app {
		t.Errorf("expected %s got %s", app, config.App)
	}
}

func TestFallbackSecrets(t *testing.T) {
	type DummySecret struct {
		Password string `json:"password"`
	}

	ak := "ak"
	sk := "sk"
	password := "supersecret"

	secretsMap := map[string]string{
		"password": password,
	}

	encSecrets, err := EncryptSecretForTest(ak, sk, secretsMap)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test-secret.sailor.fall" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(encSecrets)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	os.Setenv(ENV_SAILOR_FALLBACK_BASE_URL, server.URL)
	defer os.Unsetenv(ENV_SAILOR_FALLBACK_BASE_URL)

	consumer, err := NewConsumer[any, DummySecret](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.SECRETS,
					Path: "/invalid/path/for/volume",
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
				FallbackEnabled: true,
			},
		},
		Connection: &opts.ConnectionOption{
			Addr:          "http://localhost:7766",
			Namespace:     "test",
			App:           "test",
			AccessKey:     ak,
			SecretKey:     sk,
			SocketTimeout: time.Second * 5,
		},
	})

	if err != nil {
		t.Error(err)
		return
	}

	if err = consumer.Start(); err != nil {
		t.Error(err)
		return
	}

	secret, err := consumer.GetSecret()
	if err != nil {
		t.Error(err)
		return
	}

	if secret.Password != password {
		t.Errorf("expected %s got %s", password, secret.Password)
	}
}

func TestFallbackMisc(t *testing.T) {
	miscContent := "misc data fallback"
	resourceName := "my-misc-fallback"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test-misc.sailor.fall" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(miscContent))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	os.Setenv(ENV_SAILOR_FALLBACK_BASE_URL, server.URL)
	defer os.Unsetenv(ENV_SAILOR_FALLBACK_BASE_URL)

	consumer, err := NewConsumer[any, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.MISC,
					Name: resourceName,
					Path: "/invalid/path/for/volume",
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
				FallbackEnabled: true,
			},
		},
		Connection: &opts.ConnectionOption{
			Addr:          "http://localhost:7766",
			Namespace:     "test",
			App:           "test",
			AccessKey:     "ak",
			SecretKey:     "sk",
			SocketTimeout: time.Second * 5,
		},
	})

	if err != nil {
		t.Error(err)
		return
	}

	if err = consumer.Start(); err != nil {
		t.Error(err)
		return
	}

	misc, err := consumer.GetMisc(resourceName)
	if err != nil {
		t.Error(err)
		return
	}

	if string(misc) != miscContent {
		t.Errorf("expected %s got %s", miscContent, string(misc))
	}
}

func TestPullFallbackConfig(t *testing.T) {
	type DummyConfig struct {
		App string `json:"app"`
	}

	app := "sailor-sailing-the-containers-pull-fallback"

	// Mock server that returns 500 for the main API and success for fallback
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test-config.sailor.fall" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(DummyConfig{App: app})
			return
		}
		if r.URL.Path == "/api/v1/resource/test/test/config" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	os.Setenv(ENV_SAILOR_FALLBACK_BASE_URL, server.URL)
	defer os.Unsetenv(ENV_SAILOR_FALLBACK_BASE_URL)

	consumer, err := NewConsumer[DummyConfig, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.PULL,
					Once:  true, // Just pull once for the test
				},
				FallbackEnabled: true,
			},
		},
		Connection: &opts.ConnectionOption{
			Addr:          server.URL,
			Namespace:     "test",
			App:           "test",
			AccessKey:     "ak",
			SecretKey:     "sk",
			SocketTimeout: time.Second * 5,
		},
	})

	if err != nil {
		t.Error(err)
		return
	}

	if err = consumer.Start(); err != nil {
		t.Error(err)
		return
	}

	config, err := consumer.Get()
	if err != nil {
		t.Error(err)
		return
	}

	if config.App != app {
		t.Errorf("expected %s got %s", app, config.App)
	}
}

func TestPullFallbackSecrets(t *testing.T) {
	type DummySecret struct {
		Password string `json:"password"`
	}

	ak := "ak"
	sk := "sk"
	password := "supersecret"

	secretsMap := map[string]string{
		"password": password,
	}

	encSecrets, err := EncryptSecretForTest(ak, sk, secretsMap)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test-secret.sailor.fall" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(encSecrets)
			return
		}
		if r.URL.Path == "/api/v1/resource/test/test/secret" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	os.Setenv(ENV_SAILOR_FALLBACK_BASE_URL, server.URL)
	defer os.Unsetenv(ENV_SAILOR_FALLBACK_BASE_URL)

	consumer, err := NewConsumer[any, DummySecret](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.SECRETS,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.PULL,
					Once:  true,
				},
				FallbackEnabled: true,
			},
		},
		Connection: &opts.ConnectionOption{
			Addr:          server.URL,
			Namespace:     "test",
			App:           "test",
			AccessKey:     ak,
			SecretKey:     sk,
			SocketTimeout: time.Second * 5,
		},
	})

	if err != nil {
		t.Error(err)
		return
	}

	if err = consumer.Start(); err != nil {
		t.Error(err)
		return
	}

	secret, err := consumer.GetSecret()
	if err != nil {
		t.Error(err)
		return
	}

	if secret.Password != password {
		t.Errorf("expected %s got %s", password, secret.Password)
	}
}

func TestPullFallbackMisc(t *testing.T) {
	miscContent := "misc data fallback"
	resourceName := "my-misc-fallback"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test-misc.sailor.fall" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(miscContent))
			return
		}
		if r.URL.Path == "/api/v1/resource/test/test/misc/" + resourceName {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	os.Setenv(ENV_SAILOR_FALLBACK_BASE_URL, server.URL)
	defer os.Unsetenv(ENV_SAILOR_FALLBACK_BASE_URL)

	consumer, err := NewConsumer[any, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.MISC,
					Name: resourceName,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.PULL,
					Once:  true,
				},
				FallbackEnabled: true,
			},
		},
		Connection: &opts.ConnectionOption{
			Addr:          server.URL,
			Namespace:     "test",
			App:           "test",
			AccessKey:     "ak",
			SecretKey:     "sk",
			SocketTimeout: time.Second * 5,
		},
	})

	if err != nil {
		t.Error(err)
		return
	}

	if err = consumer.Start(); err != nil {
		t.Error(err)
		return
	}

	misc, err := consumer.GetMisc(resourceName)
	if err != nil {
		t.Error(err)
		return
	}

	if string(misc) != miscContent {
		t.Errorf("expected %s got %s", miscContent, string(misc))
	}
}

func TestPullFallbackError(t *testing.T) {
	// this will test the fallback fetch failure
	type DummyConfig struct {
		App string `json:"app"`
	}

	consumer, err := NewConsumer[DummyConfig, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.PULL,
					Once:  true,
				},
				FallbackEnabled: true,
			},
		},
		Connection: &opts.ConnectionOption{
			Addr:          "http://localhost:1234", // unreachable
			Namespace:     "test",
			App:           "test",
			AccessKey:     "ak",
			SecretKey:     "sk",
			SocketTimeout: time.Second * 5,
		},
	})

	if err != nil {
		t.Error(err)
		return
	}

	// mock fallback not set or unreachable
	err = consumer.Start()
	if err == nil {
		t.Error("expected error but got nil")
	}
}

func TestFetchFallbackServerError(t *testing.T) {
	type DummyConfig struct {
		App string `json:"app"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // Simulate failure
	}))
	defer server.Close()

	os.Setenv(ENV_SAILOR_FALLBACK_BASE_URL, server.URL)
	defer os.Unsetenv(ENV_SAILOR_FALLBACK_BASE_URL)

	consumer, err := NewConsumer[DummyConfig, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.PULL,
					Once:  true,
				},
				FallbackEnabled: true,
			},
		},
		Connection: &opts.ConnectionOption{
			Addr:          "http://localhost:1234", // unreachable
			Namespace:     "test",
			App:           "test",
			AccessKey:     "ak",
			SecretKey:     "sk",
			SocketTimeout: time.Second * 5,
		},
	})

	if err != nil {
		t.Error(err)
		return
	}

	err = consumer.Start()
	// Just executing for coverage
}
