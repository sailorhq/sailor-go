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

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

type localSailorEnv struct {
	Name string `json:"name"`
	Host string `json:"host"`
}

type localSailorConfig struct {
	Manifest struct {
		Envs []localSailorEnv `json:"envs"`
	} `json:"manifest"`
	Env   string `json:"env"`
	Token string `json:"token"`
	User  string `json:"user"`
}

func buildConnectionFromLocalConfig(base *opts.ConnectionOption) (*opts.ConnectionOption, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, ErrLocalConfigNotFound
	}

	configPath := filepath.Join(home, ".sailor", "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, ErrLocalConfigNotFound
	}

	var cfg localSailorConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, ErrLocalConfigInvalid
	}

	var host string
	for _, env := range cfg.Manifest.Envs {
		if env.Name == cfg.Env {
			host = env.Host
			break
		}
	}
	if host == "" {
		return nil, ErrLocalConfigEnvNotFound
	}

	return &opts.ConnectionOption{
		Namespace: base.Namespace,
		App:       base.App,
		Addr:      host,
		Token:     cfg.Token,
		Env:       cfg.Env,
	}, nil
}
