// sailor-go
// Copyright (C) 2025 SailorHQ and Ashish Shekar (codekidX)

// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.
package sailor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

const (
	testFolder = "./_tests"
)

func createTestFile(data any, name string) {
	filePath := path.Join(testFolder, name)
	b, _ := json.Marshal(&data)
	os.WriteFile(filePath, b, 0755)
}

func removeTestFile(name string) {
	filePath := path.Join(testFolder, name)
	os.Remove(filePath)
}

func TestNewConsumerNoResources(t *testing.T) {
	_, err := NewConsumer[any, any](opts.InitOption{})

	if !errors.Is(err, ErrNewConsumerEmptyResourceList) {
		t.Error(err)
	}
}

func TestNewConsumerInvalidConnectionOption(t *testing.T) {
	initOpts := opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
					Path: testFolder,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
			},
		},
	}
	// no connection options
	_, err := NewConsumer[any, any](initOpts)
	if !errors.Is(err, ErrNewConsumerNoSailorURL) {
		t.Error(err)
		return
	}

	var connectionOption = opts.ConnectionOption{
		Addr: "addr",
	}
	initOpts.Connection = &connectionOption

	// only addr given
	if _, err := NewConsumer[any, any](initOpts); !errors.Is(err, ErrNewConsumerNoSailorNS) {
		t.Error(err)
		return
	}

	connectionOption.Namespace = "ns"
	// only addr and ns given
	if _, err := NewConsumer[any, any](initOpts); !errors.Is(err, ErrNewConsumerNoSailorApp) {
		t.Error(err)
		return
	}

	connectionOption.App = "app"
	// only addr, ns & app given
	if _, err := NewConsumer[any, any](initOpts); !errors.Is(err, ErrNewConsumerNoSailorAccessKey) {
		t.Error(err)
		return
	}

	connectionOption.AccessKey = "ak"
	// only addr, ns, app & access key given
	if _, err := NewConsumer[any, any](initOpts); !errors.Is(err, ErrNewConsumerNoSailorSecretKey) {
		t.Error(err)
		return
	}

	connectionOption.SecretKey = "sk"
	// all options given, should not error
	if _, err := NewConsumer[any, any](initOpts); err != nil {
		t.Error(err)
	}
}

func TestVolumeConfigFileNotPresent(t *testing.T) {
	consumer, err := NewConsumer[any, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
					Path: testFolder,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
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
	// when provided all options NewConsumer function should not error
	if err != nil {
		t.Error(err)
		return
	}

	// this should error because there is no file named _config
	// inside the testFolder, it should try calling fallback and
	// fallback error should be returned
	if !errors.Is(consumer.Start(), ErrFetchFallbackFailed) {
		t.Error(err)
		return
	}
}

func TestVolumeConfigWrongType(t *testing.T) {
	type DummyConfig struct {
		App string `json:"app"`
	}

	createTestFile([]string{"yo", "lo"}, "_config")
	defer removeTestFile("_config")

	consumer, err := NewConsumer[DummyConfig, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
					Path: testFolder,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
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

	// when provided all options NewConsumer function should not error
	if err != nil {
		t.Error(err)
		return
	}

	// this should throw an error because the json parsing failed
	if consumer.Start() == nil {
		t.Error(err)
	}
}

func TestVolumeConfigCorrectData(t *testing.T) {
	type DummyConfig struct {
		App string `json:"app"`
	}

	app := "sailor-sailing-the-containers"
	createTestFile(DummyConfig{App: app}, "_config")
	defer removeTestFile("_config")

	consumer, err := NewConsumer[DummyConfig, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
					Path: testFolder,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
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

	// when provided all options NewConsumer function should not error
	if err != nil {
		t.Error(err)
		return
	}

	// this should not return an error because now the data is of correct
	// type
	if err = consumer.Start(); err != nil {
		t.Error(err)
		return
	}

	// this should not throw an error because it is of correct type
	config, err := consumer.Get()
	if err != nil {
		t.Error(err)
		return
	}

	if config.App != app {
		t.Error(fmt.Errorf("required %s got %s", app, config.App))
	}
}

func TestParseURI(t *testing.T) {
	opts, err := parseURI("sailor://a:b@localhost/ns/app")
	if err != nil {
		t.Error(err)
		return
	}

	if opts.Addr != "localhost" {
		t.Errorf("the address of the URI must be %s but got %s", "localhost", opts.Addr)
	}

	if opts.AccessKey != "a" {
		t.Errorf("the access key of the URI must be %s but got %s", "a", opts.AccessKey)
	}

	if opts.SecretKey != "b" {
		t.Errorf("the secret key of the URI must be %s but got %s", "b", opts.SecretKey)
	}

	if opts.Namespace != "ns" {
		t.Errorf("the namespace of the URI must be %s but got %s", "ns", opts.Namespace)
	}

	if opts.App != "app" {
		t.Errorf("the app of the URI must be %s but got %s", "app", opts.App)
	}
}

func TestNewConsumerWithURIFromEnv(t *testing.T) {
	// Save original env value
	originalURI := os.Getenv(ENV_SAILOR_URI)
	defer os.Setenv(ENV_SAILOR_URI, originalURI)

	// Set URI in environment
	testURI := "sailor://access:secret@localhost:7766/testns/testapp"
	os.Setenv(ENV_SAILOR_URI, testURI)

	consumer, err := NewConsumer[any, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
					Path: testFolder,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
			},
		},
		Connection: &opts.ConnectionOption{
			URI: testURI,
		},
	})

	if err != nil {
		t.Errorf("NewConsumer should not error with valid URI from env, got: %v", err)
		return
	}

	if consumer == nil {
		t.Error("NewConsumer should return a consumer")
		return
	}

	if consumer.opts.Connection == nil {
		t.Error("Connection should be set")
		return
	}

	// Verify parsed connection options
	if consumer.opts.Connection.Addr != "localhost:7766" {
		t.Errorf("expected Addr to be 'localhost:7766', got '%s'", consumer.opts.Connection.Addr)
	}

	if consumer.opts.Connection.Namespace != "testns" {
		t.Errorf("expected Namespace to be 'testns', got '%s'", consumer.opts.Connection.Namespace)
	}

	if consumer.opts.Connection.App != "testapp" {
		t.Errorf("expected App to be 'testapp', got '%s'", consumer.opts.Connection.App)
	}

	if consumer.opts.Connection.AccessKey != "access" {
		t.Errorf("expected AccessKey to be 'access', got '%s'", consumer.opts.Connection.AccessKey)
	}

	if consumer.opts.Connection.SecretKey != "secret" {
		t.Errorf("expected SecretKey to be 'secret', got '%s'", consumer.opts.Connection.SecretKey)
	}
}

func TestNewConsumerWithURIFromInitOpts(t *testing.T) {
	// Save original env value
	originalURI := os.Getenv(ENV_SAILOR_URI)
	defer os.Setenv(ENV_SAILOR_URI, originalURI)

	// Unset env variable
	os.Unsetenv(ENV_SAILOR_URI)

	testURI := "sailor://ak:sk@example.com:8080/myns/myapp"
	consumer, err := NewConsumer[any, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
					Path: testFolder,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
			},
		},
		Connection: &opts.ConnectionOption{
			URI: testURI,
		},
	})

	if err != nil {
		t.Errorf("NewConsumer should not error with valid URI from initOpts, got: %v", err)
		return
	}

	if consumer == nil {
		t.Error("NewConsumer should return a consumer")
		return
	}

	if consumer.opts.Connection == nil {
		t.Error("Connection should be set")
		return
	}

	// Verify parsed connection options
	if consumer.opts.Connection.Addr != "example.com:8080" {
		t.Errorf("expected Addr to be 'example.com:8080', got '%s'", consumer.opts.Connection.Addr)
	}

	if consumer.opts.Connection.Namespace != "myns" {
		t.Errorf("expected Namespace to be 'myns', got '%s'", consumer.opts.Connection.Namespace)
	}

	if consumer.opts.Connection.App != "myapp" {
		t.Errorf("expected App to be 'myapp', got '%s'", consumer.opts.Connection.App)
	}

	if consumer.opts.Connection.AccessKey != "ak" {
		t.Errorf("expected AccessKey to be 'ak', got '%s'", consumer.opts.Connection.AccessKey)
	}

	if consumer.opts.Connection.SecretKey != "sk" {
		t.Errorf("expected SecretKey to be 'sk', got '%s'", consumer.opts.Connection.SecretKey)
	}
}

func TestNewConsumerWithInvalidURIScheme(t *testing.T) {
	// Save original env value
	originalURI := os.Getenv(ENV_SAILOR_URI)
	defer os.Setenv(ENV_SAILOR_URI, originalURI)

	// Unset env variable
	os.Unsetenv(ENV_SAILOR_URI)

	invalidURI := "http://ak:sk@example.com/myns/myapp"
	_, err := NewConsumer[any, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
					Path: testFolder,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
			},
		},
		Connection: &opts.ConnectionOption{
			URI: invalidURI,
		},
	})

	if err == nil {
		t.Error("NewConsumer should error with invalid URI scheme")
		return
	}

	if !errors.Is(err, ErrInvalidURIPrefix) {
		t.Errorf("expected ErrInvalidURIPrefix, got: %v", err)
	}
}

func TestNewConsumerWithInvalidURIMissingPathComponents(t *testing.T) {
	// Save original env value
	originalURI := os.Getenv(ENV_SAILOR_URI)
	defer os.Setenv(ENV_SAILOR_URI, originalURI)

	// Unset env variable
	os.Unsetenv(ENV_SAILOR_URI)

	invalidURI := "sailor://ak:sk@example.com/myns"
	_, err := NewConsumer[any, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
					Path: testFolder,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
			},
		},
		Connection: &opts.ConnectionOption{
			URI: invalidURI,
		},
	})

	if err == nil {
		t.Error("NewConsumer should error with URI missing path components")
		return
	}

	if !errors.Is(err, ErrMissingURIPathComponents) {
		t.Errorf("expected ErrMissingURIPathComponents, got: %v", err)
	}
}

func TestNewConsumerWithInvalidURIMissingAccessKey(t *testing.T) {
	// Save original env value
	originalURI := os.Getenv(ENV_SAILOR_URI)
	defer os.Setenv(ENV_SAILOR_URI, originalURI)

	// Unset env variable
	os.Unsetenv(ENV_SAILOR_URI)

	invalidURI := "sailor://:sk@example.com/myns/myapp"
	_, err := NewConsumer[any, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
					Path: testFolder,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
			},
		},
		Connection: &opts.ConnectionOption{
			URI: invalidURI,
		},
	})

	if err == nil {
		t.Error("NewConsumer should error with URI missing access key")
		return
	}

	if !errors.Is(err, ErrNewConsumerNoSailorAccessKey) {
		t.Errorf("expected ErrNewConsumerNoSailorAccessKey, got: %v", err)
	}
}

func TestNewConsumerWithInvalidURIMissingSecretKey(t *testing.T) {
	// Save original env value
	originalURI := os.Getenv(ENV_SAILOR_URI)
	defer os.Setenv(ENV_SAILOR_URI, originalURI)

	// Unset env variable
	os.Unsetenv(ENV_SAILOR_URI)

	invalidURI := "sailor://ak@example.com/myns/myapp"
	_, err := NewConsumer[any, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
					Path: testFolder,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
			},
		},
		Connection: &opts.ConnectionOption{
			URI: invalidURI,
		},
	})

	if err == nil {
		t.Error("NewConsumer should error with URI missing secret key")
		return
	}

	if !errors.Is(err, ErrNewConsumerNoSailorSecretKey) {
		t.Errorf("expected ErrNewConsumerNoSailorSecretKey, got: %v", err)
	}
}

func TestNewConsumerWithURIEnvTakesPrecedence(t *testing.T) {
	// Save original env value
	originalURI := os.Getenv(ENV_SAILOR_URI)
	defer os.Setenv(ENV_SAILOR_URI, originalURI)

	// Set URI in environment
	envURI := "sailor://envak:envsk@envhost:9999/envns/envapp"
	os.Setenv(ENV_SAILOR_URI, envURI)

	// Also provide URI in initOpts (should be ignored if env is set)
	optsURI := "sailor://optak:optsk@opthost:8888/optns/optapp"
	consumer, err := NewConsumer[any, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
					Path: testFolder,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
			},
		},
		Connection: &opts.ConnectionOption{
			URI: optsURI,
		},
	})

	if err != nil {
		t.Errorf("NewConsumer should not error, got: %v", err)
		return
	}

	if consumer == nil {
		t.Error("NewConsumer should return a consumer")
		return
	}

	if consumer.opts.Connection == nil {
		t.Error("Connection should be set")
		return
	}

	// Verify that env URI takes precedence over initOpts URI
	// The env URI should be parsed, not the optsURI
	if consumer.opts.Connection.Addr != "envhost:9999" {
		t.Errorf("expected Addr to be 'envhost:9999' (from env), got '%s'", consumer.opts.Connection.Addr)
	}

	if consumer.opts.Connection.Namespace != "envns" {
		t.Errorf("expected Namespace to be 'envns' (from env), got '%s'", consumer.opts.Connection.Namespace)
	}

	if consumer.opts.Connection.App != "envapp" {
		t.Errorf("expected App to be 'envapp' (from env), got '%s'", consumer.opts.Connection.App)
	}

	if consumer.opts.Connection.AccessKey != "envak" {
		t.Errorf("expected AccessKey to be 'envak' (from env), got '%s'", consumer.opts.Connection.AccessKey)
	}

	if consumer.opts.Connection.SecretKey != "envsk" {
		t.Errorf("expected SecretKey to be 'envsk' (from env), got '%s'", consumer.opts.Connection.SecretKey)
	}
}
