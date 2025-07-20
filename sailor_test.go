package sailor

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

func TestPullConfigDefault(t *testing.T) {
	err := Initialize(opts.InitOption{
		Resources: []opts.ResourceOption{
			ConfigPullDefault(),
		},
		Connection: &opts.ConnectionOption{
			Addr:          "http://localhost:7766",
			Namespace:     "sailor",
			App:           "backend-core",
			AccessKey:     "",
			SecretKey:     "",
			SocketTimeout: time.Second * 5,
		},
	})
	if err != nil {
		t.Error(err)
		return
	}

	s := Instance()
	v, err := s.Get("app")
	if err != nil || v.(string) != "something" {
		t.Error("wrong value for app key")
	}
}

func TestVolumeConfigKeyNotPresent(t *testing.T) {
	err := Initialize(opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
					Path: "./test",
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
			AccessKey:     "",
			SecretKey:     "",
			SocketTimeout: time.Second * 5,
		},
	})
	if err != nil {
		t.Error(err)
		return
	}

	s := Instance()
	_, err = s.Get("pop")
	if err == nil {
		t.Error("should throw an error because pop key is not present")
	}
}

func TestVolumeConfig(t *testing.T) {
	err := Initialize(opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
					Path: "./test",
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
			AccessKey:     "",
			SecretKey:     "",
			SocketTimeout: time.Second * 5,
		},
	})
	if err != nil {
		t.Error(err)
		return
	}

	s := Instance()
	v, err := s.Get("app")
	if err != nil || v.(string) != "value" {
		t.Error("wrong value for app key")
	}
}

func TestVolumeConfigWithWatcherChange(t *testing.T) {
	err := Initialize(opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
					Path: "./test",
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
			},
		},
		Connection: &opts.ConnectionOption{
			Addr:          "http://localhost:7766",
			Namespace:     "test",
			App:           "test2",
			AccessKey:     "",
			SecretKey:     "",
			SocketTimeout: time.Second * 5,
		},
	})
	if err != nil {
		t.Error(err)
		return
	}

	s := Instance()
	v, err := s.Get("app")
	if err != nil || v.(string) != "value" {
		t.Error("wrong value for app key")
	}

	t.Log("changing test2-config contents!")

	newContent, _ := json.Marshal(map[string]any{"_content": `{"app": 1}`})
	os.WriteFile("test/test2-config", newContent, 0655)
	time.Sleep(1 * time.Second)

	s = Instance()
	v, err = s.Get("app")
	if err != nil || v.(float64) != 1 {
		t.Error("wrong value for app key")
		return
	}

	t.Log("reversing test2-config contents!")
	oldContent, _ := json.Marshal(map[string]any{"_content": `{"app": "value"}`})
	os.WriteFile("test/test2-config", oldContent, 0655)
}

func TestVolumeSecret(t *testing.T) {
	err := Initialize(opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.SECRETS,
					Path: "./test",
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
			},
		},
		Connection: &opts.ConnectionOption{
			Addr:          "http://localhost:7766",
			Namespace:     "test",
			App:           "test3",
			AccessKey:     "",
			SecretKey:     "",
			SocketTimeout: time.Second * 5,
		},
	})
	if err != nil {
		t.Error(err)
		return
	}

	s := Instance()
	v, err := s.GetSecret("secret")
	if err != nil || v != "shhh..." {
		t.Error("wrong value for app key")
		return
	}
}
