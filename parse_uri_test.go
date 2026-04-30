package sailor

import (
	"testing"
)

func TestParseURIErrors(t *testing.T) {
	_, err := parseURI("http://invalid-scheme")
	if err != ErrInvalidURIPrefix {
		t.Errorf("expected ErrInvalidURIPrefix, got %v", err)
	}

	_, err = parseURI("sailor://localhost/ns")
	if err != ErrMissingURIPathComponents {
		t.Errorf("expected ErrMissingURIPathComponents, got %v", err)
	}

	_, err = parseURI("sailor://ak@localhost/ns/app")
	if err != ErrNewConsumerNoSailorSecretKey {
		t.Errorf("expected ErrNewConsumerNoSailorSecretKey, got %v", err)
	}

	_, err = parseURI("sailor://localhost/ns/app")
	if err != ErrNewConsumerNoSailorAccessKey {
		t.Errorf("expected ErrNewConsumerNoSailorAccessKey, got %v", err)
	}
}

func TestParseURIInvalidURL(t *testing.T) {
	_, err := parseURI("http://%err")
	if err == nil {
		t.Errorf("expected error for invalid url, got nil")
	}
}

func TestGetMiscError(t *testing.T) {
	consumer := &Consumer[any, any]{}
	miscMap := map[string][]byte{}
	consumer.misc.Store(&miscMap)
	_, err := consumer.GetMisc("not-found")
	if err != ErrMiscNotLoaded {
		t.Errorf("expected ErrMiscNotLoaded, got %v", err)
	}
}

func TestGetErrors(t *testing.T) {
	consumer := &Consumer[any, any]{}
	_, err := consumer.Get()
	if err != ErrConfigsNotLoaded {
		t.Errorf("expected ErrConfigsNotLoaded, got %v", err)
	}

	_, err = consumer.GetSecret()
	if err != ErrSecretsNotLoaded {
		t.Errorf("expected ErrSecretsNotLoaded, got %v", err)
	}
}

