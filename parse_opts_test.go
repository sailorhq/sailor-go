package sailor

import (
	"os"
	"testing"
	"github.com/sailorhq/sailor-go/pkg/opts"
)

func TestNewConsumerRequiresURI(t *testing.T) {
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

	os.Unsetenv(ENV_SAILOR_URI)
	defer os.Unsetenv(ENV_SAILOR_URI)

	// no URI at all — must return ErrNewConsumerNoSailorURI
	_, err := NewConsumer[any, any](initOpts)
	if err != ErrNewConsumerNoSailorURI {
		t.Errorf("expected ErrNewConsumerNoSailorURI, got %v", err)
	}

	// SAILOR_URI set to a valid URI — must succeed
	os.Setenv(ENV_SAILOR_URI, "sailor://ak:sk@localhost:7766/testns/testapp")
	_, err = NewConsumer[any, any](initOpts)
	if err != nil {
		t.Errorf("expected nil error with valid SAILOR_URI, got %v", err)
	}
}
