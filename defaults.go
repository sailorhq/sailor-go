package sailor

import (
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

const (
	volume_mount_path = "/etc/sailor"
)

// ConfigMapDefault is a ResourceOption which looks for a config_map inside K8S volume
// mounted path. If it does not find the resource it fetches the resource from
// fallback.
func ConfigMapDefault() opts.ResourceOption {
	return opts.ResourceOption{
		Def: opts.ResourceDefinition{
			Kind: opts.CONFIGS,
			Path: volume_mount_path,
		},
		FetchDef: opts.FetchDefinition{
			Fetch: opts.VOLUME,
		},
		FallbackEnabled: true,
	}
}

// SecretsDefault is a ResourceOption which looks for a k8s_secret inside K8S volume
// mounted path. If it does not find the resource it fetches the resource from
// fallback.
func SecretsDefault() opts.ResourceOption {
	return opts.ResourceOption{
		Def: opts.ResourceDefinition{
			Kind: opts.SECRETS,
			Path: volume_mount_path,
		},
		FetchDef: opts.FetchDefinition{
			Fetch: opts.VOLUME,
		},
		FallbackEnabled: true,
	}
}

// MiscOnceDefault is a ResourceOption which pulls a misc config from Sailor directly.
// If it does not find the resource it fetches the resource from
// fallback.
// @NOTE: this only pulls the resource once, if you want the client to refresh on
// certain interval, use `MiscPullDefault`.
func MiscOnceDefault(resourceName string) opts.ResourceOption {
	return opts.ResourceOption{
		Def: opts.ResourceDefinition{
			Kind: opts.MISC,
			Name: resourceName,
			Path: volume_mount_path,
		},
		FetchDef: opts.FetchDefinition{
			Fetch: opts.PULL,
			Once:  true,
		},
		FallbackEnabled: true,
	}
}

// ConfigPullDefault is a ResourceOption which pulls a config from Sailor using
// pull method. This updates the config every 10 seconds.
func ConfigPullDefault() opts.ResourceOption {
	return opts.ResourceOption{
		Def: opts.ResourceDefinition{
			Kind: opts.CONFIGS,
		},
		FetchDef: opts.FetchDefinition{
			Fetch:        opts.PULL,
			PullInterval: 10 * time.Second,
		},
		FallbackEnabled: true,
	}
}

// SecretsPullDefault is a ResourceOption which pulls a secrets from Sailor using
// pull method. This updates the config every 10 seconds.
func SecretsPullDefault() opts.ResourceOption {
	return opts.ResourceOption{
		Def: opts.ResourceDefinition{
			Kind: opts.SECRETS,
		},
		FetchDef: opts.FetchDefinition{
			Fetch:        opts.PULL,
			PullInterval: 10 * time.Second,
		},
		FallbackEnabled: true,
	}
}

// MiscPullDefault is a ResourceOption which pulls a misc config from Sailor using
// pull method. This updates the config every 10 seconds.
func MiscPullDefault(resourceName string) opts.ResourceOption {
	return opts.ResourceOption{
		Def: opts.ResourceDefinition{
			Kind: opts.MISC,
			Name: resourceName,
		},
		FetchDef: opts.FetchDefinition{
			Fetch:        opts.PULL,
			PullInterval: 10 * time.Second,
		},
		FallbackEnabled: true,
	}
}
