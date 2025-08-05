// sailor-go
// Copyright (C) 2025 SailorHQ and Ashish Shekar (codekidX)

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
