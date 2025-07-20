package sailor

import (
	"time"

	"github.com/sailorhq/sailor-go/pkg/types"
)

const (
	volume_mount_path = "/etc/sailor"
)

// ConfigMapDefault is a ResourceOption which looks for a config_map inside K8S volume
// mounted path. If it does not find the resource it fetches the resource from
// fallback.
func ConfigMapDefault() types.ResourceOption {
	return types.ResourceOption{
		Def: types.ResourceDefinition{
			Kind: types.CONFIGS,
			Path: volume_mount_path,
		},
		FetchDef: types.FetchDefinition{
			Fetch: types.K8S,
		},
		FallbackEnabled: true,
	}
}

// SecretsDefault is a ResourceOption which looks for a k8s_secret inside K8S volume
// mounted path. If it does not find the resource it fetches the resource from
// fallback.
func SecretsDefault() types.ResourceOption {
	return types.ResourceOption{
		Def: types.ResourceDefinition{
			Kind: types.SECRETS,
			Path: volume_mount_path,
		},
		FetchDef: types.FetchDefinition{
			Fetch: types.K8S,
		},
		FallbackEnabled: true,
	}
}

// MiscOnceDefault is a ResourceOption which pulls a misc config from Sailor directly.
// If it does not find the resource it fetches the resource from
// fallback.
// @NOTE: this only pulls the resource once, if you want the client to refresh on
// certain interval, use `MiscPullDefault`.
func MiscOnceDefault(resourceName string) types.ResourceOption {
	return types.ResourceOption{
		Def: types.ResourceDefinition{
			Kind: types.MISC,
			Name: resourceName,
			Path: volume_mount_path,
		},
		FetchDef: types.FetchDefinition{
			Fetch: types.PULL,
			Once:  true,
		},
		FallbackEnabled: true,
	}
}

// ConfigPullDefault is a ResourceOption which pulls a config from Sailor using
// pull method. This updates the config every 10 seconds.
func ConfigPullDefault() types.ResourceOption {
	return types.ResourceOption{
		Def: types.ResourceDefinition{
			Kind: types.CONFIGS,
		},
		FetchDef: types.FetchDefinition{
			Fetch:        types.PULL,
			PullInterval: 10 * time.Second,
		},
		FallbackEnabled: true,
	}
}

// SecretsPullDefault is a ResourceOption which pulls a secrets from Sailor using
// pull method. This updates the config every 10 seconds.
func SecretsPullDefault() types.ResourceOption {
	return types.ResourceOption{
		Def: types.ResourceDefinition{
			Kind: types.SECRETS,
		},
		FetchDef: types.FetchDefinition{
			Fetch:        types.PULL,
			PullInterval: 10 * time.Second,
		},
		FallbackEnabled: true,
	}
}

// MiscPullDefault is a ResourceOption which pulls a misc config from Sailor using
// pull method. This updates the config every 10 seconds.
func MiscPullDefault(resourceName string) types.ResourceOption {
	return types.ResourceOption{
		Def: types.ResourceDefinition{
			Kind: types.MISC,
			Name: resourceName,
		},
		FetchDef: types.FetchDefinition{
			Fetch:        types.PULL,
			PullInterval: 10 * time.Second,
		},
		FallbackEnabled: true,
	}
}
