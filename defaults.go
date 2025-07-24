// sailor-go
// Copyright (C) 2025 sailorhq-codekidx

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
