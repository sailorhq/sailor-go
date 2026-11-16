package sailor

import (
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

const (
	CONFIG_MOUNT_PATH = "/etc/sailor"
	SECRET_MOUNT_PATH = "/etc/sailor/secret"
)

// Resource declares a resource decoded into T with the lang's decoder
// (SAILOR_CONSUMER.md section 4). Retrieve it with Get[T].
func Resource[T any](def opts.ResourceDefinition, fetch opts.FetchDefinition) opts.ResourceOption {
	return opts.ResourceOption{
		Def:      def,
		FetchDef: fetch,
		Decode: func(raw []byte, decode opts.DecodeFunc) (any, error) {
			var v T
			if err := decode(raw, &v); err != nil {
				return nil, err
			}
			return v, nil
		},
	}
}

// Raw declares a resource stored as bytes with no decoding. Retrieve it with Get[[]byte].
func Raw(def opts.ResourceDefinition, fetch opts.FetchDefinition) opts.ResourceOption {
	return opts.ResourceOption{Def: def, FetchDef: fetch}
}

// ConfigMapDefault reads resource "config" from the mounted volume.
func ConfigMapDefault[T any]() opts.ResourceOption {
	return Resource[T](
		opts.ResourceDefinition{Name: "config", Path: CONFIG_MOUNT_PATH},
		opts.FetchDefinition{Fetch: opts.VOLUME},
	)
}

// SecretsDefault reads the encrypted resource "secret" from the mounted volume.
func SecretsDefault[T any]() opts.ResourceOption {
	return Resource[T](
		opts.ResourceDefinition{Name: "secret", Encrypted: true, Path: SECRET_MOUNT_PATH},
		opts.FetchDefinition{Fetch: opts.VOLUME},
	)
}

// ConfigPullDefault polls resource "config" every 10 seconds.
func ConfigPullDefault[T any]() opts.ResourceOption {
	return Resource[T](
		opts.ResourceDefinition{Name: "config"},
		opts.FetchDefinition{Fetch: opts.PULL, PullInterval: 10 * time.Second},
	)
}

// SecretsPullDefault polls the encrypted resource "secret" every 10 seconds.
func SecretsPullDefault[T any]() opts.ResourceOption {
	return Resource[T](
		opts.ResourceDefinition{Name: "secret", Encrypted: true},
		opts.FetchDefinition{Fetch: opts.PULL, PullInterval: 10 * time.Second},
	)
}

// MiscPullDefault polls a named raw resource every 10 seconds.
func MiscPullDefault(name string) opts.ResourceOption {
	return Raw(
		opts.ResourceDefinition{Name: name},
		opts.FetchDefinition{Fetch: opts.PULL, PullInterval: 10 * time.Second},
	)
}

// ConfigDevDefault caches "config" under ~/.sailor/cache for local development.
func ConfigDevDefault[T any]() opts.ResourceOption {
	return Resource[T](
		opts.ResourceDefinition{Name: "config"},
		opts.FetchDefinition{Fetch: opts.DEV},
	)
}

// SecretsDevDefault caches the encrypted "secret" under ~/.sailor/cache for local development.
func SecretsDevDefault[T any]() opts.ResourceOption {
	return Resource[T](
		opts.ResourceDefinition{Name: "secret", Encrypted: true},
		opts.FetchDefinition{Fetch: opts.DEV},
	)
}
