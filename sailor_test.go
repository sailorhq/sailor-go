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
package sailor_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/sailorhq/sailor-go"
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
	_, err := sailor.NewConsumer[any, any](opts.InitOption{})

	if !errors.Is(err, sailor.ErrNewConsumerEmptyResourceList) {
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
	_, err := sailor.NewConsumer[any, any](initOpts)
	if !errors.Is(err, sailor.ErrNewConsumerNoSailorURL) {
		t.Error(err)
		return
	}

	var connectionOption = opts.ConnectionOption{
		Addr: "addr",
	}
	initOpts.Connection = &connectionOption

	// only addr given
	if _, err := sailor.NewConsumer[any, any](initOpts); !errors.Is(err, sailor.ErrNewConsumerNoSailorNS) {
		t.Error(err)
		return
	}

	connectionOption.Namespace = "ns"
	// only addr and ns given
	if _, err := sailor.NewConsumer[any, any](initOpts); !errors.Is(err, sailor.ErrNewConsumerNoSailorApp) {
		t.Error(err)
		return
	}

	connectionOption.App = "app"
	// only addr, ns & app given
	if _, err := sailor.NewConsumer[any, any](initOpts); !errors.Is(err, sailor.ErrNewConsumerNoSailorAccessKey) {
		t.Error(err)
		return
	}

	connectionOption.AccessKey = "ak"
	// only addr, ns, app & access key given
	if _, err := sailor.NewConsumer[any, any](initOpts); !errors.Is(err, sailor.ErrNewConsumerNoSailorSecretKey) {
		t.Error(err)
		return
	}

	connectionOption.SecretKey = "sk"
	// all options given, should not error
	if _, err := sailor.NewConsumer[any, any](initOpts); err != nil {
		t.Error(err)
	}
}

func TestVolumeConfigFileNotPresent(t *testing.T) {
	consumer, err := sailor.NewConsumer[any, any](opts.InitOption{
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
	if !errors.Is(consumer.Start(), sailor.ErrFetchFallbackFailed) {
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

	consumer, err := sailor.NewConsumer[DummyConfig, any](opts.InitOption{
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

	consumer, err := sailor.NewConsumer[DummyConfig, any](opts.InitOption{
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
