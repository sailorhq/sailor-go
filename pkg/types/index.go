package types

import "time"

type ResourceKind int

type FetchOption int

const (
	K8S FetchOption = iota + 1
	PULL

	CONFIGS ResourceKind = iota + 1
	SECRETS
	MISC
)

type ConnectionParam struct {
	Addr          string
	Namespace     string
	App           string
	AccessKey     string
	SecretKey     string
	SocketTimeout time.Duration
}

type SailorOpts struct {
	Connection *ConnectionParam
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
