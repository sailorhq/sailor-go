package sailor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

func TestPullConfig(t *testing.T) {
	type DummyConfig struct {
		App string `json:"app"`
	}

	app := "sailor-sailing-the-containers"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/resource/test/test/config" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(DummyConfig{App: app})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

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

func TestPullMisc(t *testing.T) {
	miscContent := "misc data pull"
	resourceName := "my-misc-pull"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/resource/test/test/misc/" + resourceName {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(miscContent))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

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

func TestPullSecrets(t *testing.T) {
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
		if r.URL.Path == "/api/v1/resource/test/test/secret" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(encSecrets)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

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
