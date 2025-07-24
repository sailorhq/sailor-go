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
package opts

import "time"

type ResourceKind string

type FetchOption int

const (
	VOLUME FetchOption = iota + 1
	PULL

	CONFIGS ResourceKind = "config"
	SECRETS ResourceKind = "secret"
	MISC    ResourceKind = "misc"
)

type ConnectionOption struct {
	Addr          string
	Namespace     string
	App           string
	AccessKey     string
	SecretKey     string
	SocketTimeout time.Duration
}

type InitOption struct {
	Connection *ConnectionOption
	Logging    bool

	// Resources defines what all resources does the Sailor Client need to manage
	Resources []ResourceOption
}

type ResourceDefinition struct {
	Kind ResourceKind
	Name string
	Path string
}

type FetchDefinition struct {
	// Fetch defines how and from where to fetch the resource
	Fetch FetchOption

	// Once tells client not to fetch the config more than once
	// So for resources like ConfigMap & Secrets there will be no informer attached
	// And for Misc resource we will fetch only once from Sailor
	Once bool

	// PullInterval is only used for FetchOption.Pull and defaults to 10 seconds
	// if not passed during Resource Definition
	PullInterval time.Duration
}

type ResourceOption struct {
	Def             ResourceDefinition
	FetchDef        FetchDefinition
	FallbackEnabled bool
}

type SailorMeta struct {
	Version string `json:"version"`
}

type SailorState struct {
	Version string            `json:"config_ver"`
	Config  []byte            `json:"config"`
	Secrets map[string][]byte `json:"secrets"`
}
