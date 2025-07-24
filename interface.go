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
	"fmt"
)

type ISailor interface {
	Get(key string) (any, error)
	GetSecret(key string) (string, error)
	GetMisc(resourceName string) (string, error)
}

type SailorInstance struct {
	config  *map[string]any
	secrets *map[string]string
	misc    *map[string]string
}

func (s *SailorInstance) Get(key string) (any, error) {
	var data any
	var ok bool
	if data, ok = (*s.config)[key]; !ok {
		return nil, fmt.Errorf("config key %s not found", key)
	}
	return data, nil
}

func (s *SailorInstance) GetSecret(key string) (string, error) {
	var data string
	var ok bool
	if data, ok = (*s.secrets)[key]; !ok {
		return "", fmt.Errorf("secret key %s not found", key)
	}

	return data, nil
}

func (s *SailorInstance) GetMisc(resourceName string) (string, error) {
	var data string
	var ok bool
	if data, ok = (*s.misc)[resourceName]; !ok {
		return "", fmt.Errorf("misc resource %s not found", resourceName)
	}

	return data, nil
}

func Instance() ISailor {
	return &SailorInstance{
		config:  consumer.configs.Load().(*map[string]any),
		secrets: consumer.secrets.Load().(*map[string]string),
		misc:    consumer.misc.Load().(*map[string]string),
	}
}
