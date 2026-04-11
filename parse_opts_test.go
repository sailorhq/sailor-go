package sailor

import (
	"os"
	"testing"
	"github.com/sailorhq/sailor-go/pkg/opts"
)

func TestNewConsumerEnvVars(t *testing.T) {
	initOpts := opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.CONFIGS,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
			},
		},
	}

	os.Unsetenv(ENV_SAILOR_URL)
	defer os.Unsetenv(ENV_SAILOR_URL)
	os.Unsetenv(ENV_SAILOR_NS)
	defer os.Unsetenv(ENV_SAILOR_NS)
	os.Unsetenv(ENV_SAILOR_APP)
	defer os.Unsetenv(ENV_SAILOR_APP)
	os.Unsetenv(ENV_SAILOR_ACCESS_KEY)
	defer os.Unsetenv(ENV_SAILOR_ACCESS_KEY)
	os.Unsetenv(ENV_SAILOR_SECRET_KEY)
	defer os.Unsetenv(ENV_SAILOR_SECRET_KEY)

	_, err := NewConsumer[any, any](initOpts)
	if err != ErrNewConsumerNoSailorURL {
		t.Errorf("expected ErrNewConsumerNoSailorURL, got %v", err)
	}

	os.Setenv(ENV_SAILOR_URL, "http://localhost")
	_, err = NewConsumer[any, any](initOpts)
	if err != ErrNewConsumerNoSailorNS {
		t.Errorf("expected ErrNewConsumerNoSailorNS, got %v", err)
	}

	os.Setenv(ENV_SAILOR_NS, "ns")
	_, err = NewConsumer[any, any](initOpts)
	if err != ErrNewConsumerNoSailorApp {
		t.Errorf("expected ErrNewConsumerNoSailorApp, got %v", err)
	}

	os.Setenv(ENV_SAILOR_APP, "app")
	_, err = NewConsumer[any, any](initOpts)
	if err != ErrNewConsumerNoSailorAccessKey {
		t.Errorf("expected ErrNewConsumerNoSailorAccessKey, got %v", err)
	}

	os.Setenv(ENV_SAILOR_ACCESS_KEY, "ak")
	_, err = NewConsumer[any, any](initOpts)
	if err != ErrNewConsumerNoSailorSecretKey {
		t.Errorf("expected ErrNewConsumerNoSailorSecretKey, got %v", err)
	}

	os.Setenv(ENV_SAILOR_SECRET_KEY, "sk")
	_, err = NewConsumer[any, any](initOpts)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}
