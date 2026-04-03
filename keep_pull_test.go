package sailor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

func TestKeepPullingResource(t *testing.T) {
	type DummyConfig struct {
		App string `json:"app"`
	}

	app := "sailor-app"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(DummyConfig{App: app})
	}))
	defer server.Close()

	consumer, err := NewConsumer[DummyConfig, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
				},
				FetchDef: opts.FetchDefinition{
					Fetch:        opts.PULL,
					Once:         false,
					PullInterval: time.Millisecond * 10,
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

	if err := consumer.Start(); err != nil {
		t.Error(err)
		return
	}

	time.Sleep(time.Millisecond * 50)

	config, err := consumer.Get()
	if err != nil {
		t.Error(err)
		return
	}

	if config.App != app {
		t.Errorf("expected %s got %s", app, config.App)
	}
}

func TestKeepPullingResourceSecrets(t *testing.T) {
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
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(encSecrets)
	}))
	defer server.Close()

	consumer, err := NewConsumer[any, DummySecret](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.SECRETS,
				},
				FetchDef: opts.FetchDefinition{
					Fetch:        opts.PULL,
					Once:         false,
					PullInterval: time.Millisecond * 10,
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

	if err := consumer.Start(); err != nil {
		t.Error(err)
		return
	}

	time.Sleep(time.Millisecond * 50)

	secret, err := consumer.GetSecret()
	if err != nil {
		t.Error(err)
		return
	}

	if secret.Password != password {
		t.Errorf("expected %s got %s", password, secret.Password)
	}
}

func TestKeepPullingResourceMisc(t *testing.T) {
	miscContent := "misc data keep pull"
	resourceName := "my-misc-keep-pull"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(miscContent))
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
					Fetch:        opts.PULL,
					Once:         false,
					PullInterval: time.Millisecond * 10,
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

	if err := consumer.Start(); err != nil {
		t.Error(err)
		return
	}

	time.Sleep(time.Millisecond * 50)

	misc, err := consumer.GetMisc(resourceName)
	if err != nil {
		t.Error(err)
		return
	}

	if string(misc) != miscContent {
		t.Errorf("expected %s got %s", miscContent, string(misc))
	}
}

func TestKeepPullingResourceErrors(t *testing.T) {
	type DummyConfig struct {
		App string `json:"app"`
	}

	app := "sailor-app"

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(DummyConfig{App: app})
	}))
	defer server.Close()

	consumer, err := NewConsumer[DummyConfig, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
				},
				FetchDef: opts.FetchDefinition{
					Fetch:        opts.PULL,
					Once:         false,
					PullInterval: time.Millisecond * 10,
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

	if err := consumer.Start(); err != nil {
		t.Error(err)
		return
	}

	time.Sleep(time.Millisecond * 50)
}
