package sailor

import "errors"

var (
	ErrNewConsumerEmptyResourceList = errors.New("no resources to manage, pass Resources inside opts")
	ErrNewConsumerNoSailorURL       = errors.New("cannot connect to sailor without address, either pass ENV_SAILOR_URL or set Connection in SailorOpts")
	ErrNewConsumerNoSailorNS        = errors.New("cannot connect to sailor without address, either pass ENV_SAILOR_NS or set Connection in SailorOpts")
	ErrNewConsumerNoSailorApp       = errors.New("cannot connect to sailor without address, either pass ENV_SAILOR_APP or set Connection in SailorOpts")
	ErrNewConsumerNoSailorAccessKey = errors.New("cannot connect to sailor without address, either pass ENV_SAILOR_ACCESS_KEY or set Connection in SailorOpts")
	ErrNewConsumerNoSailorSecretKey = errors.New("cannot connect to sailor without address, either pass ENV_SAILOR_SECRET_KEY or set Connection in SailorOpts")
	ErrFetchFallbackFailed          = errors.New("cannot find config to serve, fallback fetch also failed")
	ErrConfigsNotLoaded             = errors.New("configs are not loaded")
	ErrSecretsNotLoaded             = errors.New("secrets are not loaded")
	ErrMiscNotLoaded                = errors.New("misc resource are not loaded")
)
