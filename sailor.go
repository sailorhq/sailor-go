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
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
	"github.com/sailorhq/sailor/pkg/vault"

	"github.com/fsnotify/fsnotify"
)

const (
	ENV_SAILOR_URL               = "SAILOR_URL"
	ENV_SAILOR_NS                = "SAILOR_NS"
	ENV_SAILOR_APP               = "SAILOR_APP"
	ENV_SAILOR_ACCESS_KEY        = "SAILOR_ACCESS_KEY"
	ENV_SAILOR_SECRET_KEY        = "SAILOR_SECRET_KEY"
	ENV_SAILOR_FALLBACK_BASE_URL = "SAILOR_FALLBACK_BASE_URL"
)

type Consumer[C any, S any] struct {
	opts opts.InitOption

	sailorClient *http.Client

	// configs are values which represent ConfigMap or AppConfig
	configs atomic.Pointer[C]

	// secrets corresponds to secret resource for this app inside the namespace
	secrets atomic.Pointer[S]

	// misc corresponds to misc resource which can be any text based file format
	// @NOTE: the caller consuming this resource must know the type and how to
	// make sense of them
	misc atomic.Pointer[map[string][]byte]

	// watcher is a file watcher for any watchable resource defined during
	// connection with a ResourceOption
	watcher *fsnotify.Watcher

	// hasWatchableResource says to the consumer to init watcher only if
	// there is any watchable resource defined, for example k8s ConfigMap
	hasWatchableResource bool
}

// watcherInfo is a union of the resource which needs to be watched
type watcherInfo struct {
	kind opts.ResourceKind

	// path is the directory where the resource can be found
	path string

	// name is the name of the resource, this is only used in case of misc config
	// where a resource can have its own name
	name string
}

// watcherFileNameResourceMap keeps tab of resources which needs to be watched.
// @key = the name of the resource
// @value = metadata of the value
var watcherFileNameResourceMap = map[string]watcherInfo{}

// NewConsumer function initializes the sailor consumer with the given ResourceOption(s).
// where:
//
//	C = type of the config which is used while binding.
//	S = type of secret which is used while binding. Note that the secrets are always
//	    of type map[string]string it is included in this method only for readability
//	    and ease of use.
//
// If the ResourceOption(s) are empty, sailor doesn't consume anything.
// You can either provied ConnectionParam through `opts` or through ENV variables.
// If both of them are empty, sailor doesn't consume anything.
func NewConsumer[C any, S any](initOpts opts.InitOption) (*Consumer[C, S], error) {
	var consumer Consumer[C, S]
	if len(initOpts.Resources) == 0 {
		return nil, ErrNewConsumerEmptyResourceList
	}

	if initOpts.Connection == nil {
		var conn = opts.ConnectionOption{}
		// we try getting all the necessary details from env

		if conn.Addr = os.Getenv(ENV_SAILOR_URL); conn.Addr == "" {
			return nil, ErrNewConsumerNoSailorURL
		}

		if conn.Namespace = os.Getenv(ENV_SAILOR_NS); conn.Namespace == "" {
			return nil, ErrNewConsumerNoSailorNS
		}

		if conn.App = os.Getenv(ENV_SAILOR_APP); conn.App == "" {
			return nil, ErrNewConsumerNoSailorApp
		}

		if conn.AccessKey = os.Getenv(ENV_SAILOR_ACCESS_KEY); conn.AccessKey == "" {
			return nil, ErrNewConsumerNoSailorAccessKey
		}

		if conn.SecretKey = os.Getenv(ENV_SAILOR_SECRET_KEY); conn.SecretKey == "" {
			return nil, ErrNewConsumerNoSailorSecretKey
		}
	} else {
		if initOpts.Connection.Addr == "" {
			return nil, ErrNewConsumerNoSailorURL
		}

		if initOpts.Connection.Namespace == "" {
			return nil, ErrNewConsumerNoSailorNS
		}

		if initOpts.Connection.App == "" {
			return nil, ErrNewConsumerNoSailorApp
		}

		if initOpts.Connection.AccessKey == "" {
			return nil, ErrNewConsumerNoSailorAccessKey
		}

		if initOpts.Connection.SecretKey == "" {
			return nil, ErrNewConsumerNoSailorSecretKey
		}
		consumer.opts = initOpts
	}

	return &consumer, nil
}

func (c *Consumer[C, S]) Start() error {
	// TODO :: check if this is needed and if we can use atomic.Pointer here as well
	c.misc.Store(&map[string][]byte{})

	c.watcher, _ = fsnotify.NewWatcher()

	// we will check what resources are required and how to manage them
	for _, res := range c.opts.Resources {
		switch res.Def.Kind {
		case opts.CONFIGS:
			if err := c.manageConfig(&res); err != nil {
				return err
			}
		case opts.SECRETS:
			if err := c.manageSecrets(&res); err != nil {
				return err
			}
		case opts.MISC:
			if err := c.manageMisc(&res); err != nil {
				return err
			}
		}
	}

	// this means that there are volume mounted resources which needs to be watched
	// for changes
	if c.hasWatchableResource {
		go c.watchForVolumeChanges()
	}

	return nil
}

// watchForVolumeChanges checks for all the paths mentioned in ResourceOption(s)
// which is of kind: Volume.
func (c *Consumer[C, S]) watchForVolumeChanges() {
	for {
		select {
		case event := <-c.watcher.Events:
			if event.Has(fsnotify.Chmod) || event.Has(fsnotify.Write) {
				for _, wi := range watcherFileNameResourceMap {
					// TODO :: we need to keep a checksum where it computes the hash
					// and keeps it in memory for checking if the file has changed or not.
					// If it is deployed in a volume inside K8s, this uses symlink and
					// we don't come to know which resource has changed.
					switch wi.kind {
					case opts.CONFIGS:
						configBytes, err := os.ReadFile(wi.path)
						if err != nil {
							log.Println("config has changed but unable to updated it due to: ", err.Error())
							continue
						}

						if err := c.storeRawResource(configBytes, wi.kind, wi.name); err != nil {
							log.Println("config has changed but unable to store it due to: ", err.Error())
							continue
						}
					case opts.SECRETS:
						secretBytes, err := os.ReadFile(wi.path)
						if err != nil {
							log.Println("secrets has changed but unable to updated it due to: ", err.Error())
							continue
						}

						if err := c.storeRawResource(secretBytes, wi.kind, wi.name); err != nil {
							log.Println("secrets has changed but unable to store it due to: ", err.Error())
							continue
						}
					case opts.MISC:
						miscBytes, err := os.ReadFile(wi.path)
						if err != nil {
							log.Println("misc has changed but unable to updated it due to: ", err.Error())
							continue
						}

						if err := c.storeRawResource(miscBytes, wi.kind, wi.name); err != nil {
							log.Println("misc has changed but unable to store it due to: ", err.Error())
							continue
						}
					}
				}

			}
		case err := <-c.watcher.Errors:
			log.Println(err)
		}
	}
}

// manageConfig manages the config defined inside Sailor for a given namespace and app
func (c *Consumer[C, S]) manageConfig(res *opts.ResourceOption) error {
	switch res.FetchDef.Fetch {
	case opts.VOLUME:
		// check if file is present in the path
		resourcePath := fmt.Sprintf("%s/_config", res.Def.Path)
		configBytes, err := os.ReadFile(resourcePath)
		if err == nil {
			if err := c.storeRawResource(configBytes, res.Def.Kind, res.Def.Name); err != nil {
				return err
			}

			// add watcher details
			c.hasWatchableResource = true
			watcherFileNameResourceMap["_config"] = watcherInfo{opts.CONFIGS, resourcePath, ""}
			// we watch for directory changes as volume mount swaps with symlinks
			c.watcher.Add(res.Def.Path)

			return nil
		}

		if err := c.fetchFallback(res.Def.Kind, res.Def.Name); err != nil {
			return err
		}

		return nil
	case opts.PULL:
		// we will pull for the latest config with version
		url := fmt.Sprintf("%s/api/v1/resource/%s/%s/config",
			c.opts.Connection.Addr,
			c.opts.Connection.Namespace,
			c.opts.Connection.App,
		)

		resp, err := c.sailorClient.Get(url)
		if err == nil {
			if resp.StatusCode != http.StatusOK {
				// :goto fallback
				break
			}

			configBytes, err := io.ReadAll(resp.Body)
			defer resp.Body.Close()
			if err != nil {
				// :goto fallback
				break
			}

			if err := c.storeRawResource(configBytes, res.Def.Kind, res.Def.Name); err != nil {
				return err
			}

			// time to check if we want to pull the resource in background thread
			if !res.FetchDef.Once {
				go c.keepPullingResource(res)
			}

			return nil
		}

		if err := c.fetchFallback(res.Def.Kind, res.Def.Name); err != nil {
			return err
		}

		return nil
	}
	return nil
}

func (c *Consumer[C, S]) manageSecrets(res *opts.ResourceOption) error {
	switch res.FetchDef.Fetch {
	case opts.VOLUME:
		// check if file is present in the path
		resourcePath := fmt.Sprintf("%s/_secret", res.Def.Path)
		secretBytes, err := os.ReadFile(resourcePath)
		if err == nil {
			if err := c.storeRawResource(secretBytes, res.Def.Kind, res.Def.Name); err != nil {
				return err
			}

			// add watcher details
			c.hasWatchableResource = true
			watcherFileNameResourceMap["_secret"] = watcherInfo{opts.SECRETS, resourcePath, ""}
			c.watcher.Add(res.Def.Path)

			return nil
		}

		if err := c.fetchFallback(res.Def.Kind, res.Def.Name); err != nil {
			return err
		}

		return nil
	case opts.PULL:
		// we will pull for the latest config with version
		url := fmt.Sprintf("%s/api/v1/resource/%s/%s/secret",
			c.opts.Connection.Addr,
			c.opts.Connection.Namespace,
			c.opts.Connection.App,
		)

		resp, err := c.sailorClient.Get(url)
		if err == nil {
			if resp.StatusCode != http.StatusOK {
				// goto fallback
				break
			}

			secretBytes, err := io.ReadAll(resp.Body)
			defer resp.Body.Close()
			if err != nil {
				// goto fallback
				break
			}

			if err := c.storeRawResource(secretBytes, res.Def.Kind, res.Def.Name); err != nil {
				return err
			}

			// time to check if we want to pull the resource in background thread
			if !res.FetchDef.Once {
				go c.keepPullingResource(res)
			}

			return nil
		}

		if err := c.fetchFallback(res.Def.Kind, res.Def.Name); err != nil {
			return err
		}

		return nil
	}
	return nil
}

func (c *Consumer[C, S]) manageMisc(res *opts.ResourceOption) error {
	switch res.FetchDef.Fetch {
	case opts.VOLUME:
		// check if file is present in the path
		resourcePath := fmt.Sprintf("%s/_%s", res.Def.Path, res.Def.Name)
		miscBytes, err := os.ReadFile(resourcePath)
		if err == nil {
			if err := c.storeRawResource(miscBytes, res.Def.Kind, res.Def.Name); err != nil {
				return err
			}

			// add watcher details
			c.hasWatchableResource = true
			watcherFileNameResourceMap["_"+res.Def.Name] = watcherInfo{opts.MISC, resourcePath, res.Def.Name}
			c.watcher.Add(res.Def.Path)

			return nil
		}

		if err := c.fetchFallback(res.Def.Kind, res.Def.Name); err != nil {
			return err
		}

		return nil
	case opts.PULL:
		// we will pull for the latest config with version
		url := fmt.Sprintf("%s/api/v1/resource/%s/%s/misc/%s",
			c.opts.Connection.Addr,
			c.opts.Connection.Namespace,
			c.opts.Connection.App,
			res.Def.Name,
		)

		resp, err := c.sailorClient.Get(url)
		if err == nil {
			if resp.StatusCode != http.StatusOK {
				// goto fallback
				break
			}

			miscBytes, err := io.ReadAll(resp.Body)
			defer resp.Body.Close()
			if err != nil {
				// goto fallback
				break
			}

			if err := c.storeRawResource(miscBytes, res.Def.Kind, res.Def.Name); err != nil {
				return err
			}

			// time to check if we want to pull the resource in background thread
			if !res.FetchDef.Once {
				go c.keepPullingResource(res)
			}

			return nil
		}

		if err := c.fetchFallback(res.Def.Kind, res.Def.Name); err != nil {
			return err
		}

		return nil
	}
	return nil
}

func (c *Consumer[C, S]) fetchFallback(forKind opts.ResourceKind, resName string) error {
	fallbackBaseURL := os.Getenv(ENV_SAILOR_FALLBACK_BASE_URL)
	if fallbackBaseURL != "" {
		url := fmt.Sprintf("%s/%s-%s.sailor.fall", fallbackBaseURL, c.opts.Connection.App, forKind)
		resp, err := c.sailorClient.Get(url)
		if err != nil {
			return err
		}

		resBytes, err := io.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err != nil {
			return err
		}

		if err = c.storeRawResource(resBytes, forKind, resName); err != nil {
			return err
		}

		return nil
	}

	return ErrFetchFallbackFailed
}

func (c *Consumer[C, S]) keepPullingResource(res *opts.ResourceOption) {
	var url string
	switch res.Def.Kind {
	case opts.CONFIGS:
		url = fmt.Sprintf("%s/api/v1/resource/%s/%s/config",
			c.opts.Connection.Addr,
			c.opts.Connection.Namespace,
			c.opts.Connection.App,
		)
	case opts.SECRETS:
		url = fmt.Sprintf("%s/api/v1/resource/%s/%s/secret",
			c.opts.Connection.Addr,
			c.opts.Connection.Namespace,
			c.opts.Connection.App,
		)
	case opts.MISC:
		url = fmt.Sprintf("%s/api/v1/resource/%s/%s/misc/%s",
			c.opts.Connection.Addr,
			c.opts.Connection.Namespace,
			c.opts.Connection.App,
			res.Def.Name,
		)
	}

	resp, err := c.sailorClient.Get(url)
	if err == nil {
		if resp.StatusCode != http.StatusOK {
			time.Sleep(res.FetchDef.PullInterval)
			c.keepPullingResource(res)
			return
		}

		resBytes, err := io.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err != nil {
			time.Sleep(res.FetchDef.PullInterval)
			c.keepPullingResource(res)
			return
		}

		if err = c.storeRawResource(resBytes, res.Def.Kind, res.Def.Name); err != nil {
			time.Sleep(res.FetchDef.PullInterval)
			c.keepPullingResource(res)
			return
		}
	}

	time.Sleep(res.FetchDef.PullInterval)
	c.keepPullingResource(res)
}

func (c *Consumer[C, S]) storeRawResource(resBytes []byte, forKind opts.ResourceKind, resourceName string) error {
	switch forKind {
	case opts.CONFIGS:
		var config C
		if err := json.Unmarshal(resBytes, &config); err != nil {
			// TODO :: log here!
			return err
		}

		c.configs.Store(&config)
	case opts.SECRETS:
		var encSecrets map[string]vault.SecretRecord
		if err := json.Unmarshal(resBytes, &encSecrets); err != nil {
			return err
		}

		kek, err := vault.DeriveKEK(c.opts.Connection.SecretKey, []byte(c.opts.Connection.AccessKey))
		if err != nil {
			return err
		}

		var interimSecrets = make(map[string]string, len(encSecrets))
		for k, ev := range encSecrets {
			dek, err := vault.DecryptDEK(ev.EncryptedDEK, kek)
			if err != nil {
				return err
			}
			v, err := vault.DecryptWithDEK(ev.EncryptedSecret, dek)
			if err != nil {
				return err
			}

			interimSecrets[k] = v
		}

		b, err := json.Marshal(&interimSecrets)
		if err != nil {
			return err
		}

		var secrets S
		if err := json.Unmarshal(b, &secrets); err != nil {
			return err
		}

		c.secrets.Store(&secrets)
	case opts.MISC:
		miscCopy := maps.Clone(*c.misc.Load())
		miscCopy[resourceName] = resBytes
		c.misc.Store(&miscCopy)
	}

	return nil
}

// Get returns the current configuration
func (c *Consumer[C, S]) Get() (C, error) {
	configPtr := c.configs.Load()
	if configPtr == nil {
		var zero C
		return zero, ErrConfigsNotLoaded
	}

	return *configPtr, nil
}

// Get returns the current secrets
func (c *Consumer[C, S]) GetSecret() (S, error) {
	secretPtr := c.secrets.Load()
	if secretPtr == nil {
		var zero S
		return zero, ErrSecretsNotLoaded
	}

	return *secretPtr, nil
}

// Get misc resource bytes by name
func (c *Consumer[C, S]) GetMisc(name string) ([]byte, error) {
	miscMap := *c.misc.Load()
	if _, ok := miscMap[name]; !ok {
		return []byte{}, ErrMiscNotLoaded
	}

	return miscMap[name], nil
}
