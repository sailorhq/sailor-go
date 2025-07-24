package sailor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"os"
	"path"
	"sync/atomic"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"

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

type sailor struct {
	opts opts.InitOption

	sailorClient *http.Client

	// configs are values which represent ConfigMap or AppConfig
	configs atomic.Value

	// secrets corresponds to secret resource for this app inside the namespace
	secrets atomic.Value

	// misc corresponds to misc resource which can be any text based file format
	// @NOTE: the caller consuming this resource must know the type and how to
	// make sense of them
	misc atomic.Value

	// watcher is a file watcher for any watchable resource defined during
	// connection with a ResourceOption
	watcher *fsnotify.Watcher

	// hasWatchableResource says to the consumer to init watcher only if
	// there is any watchable resource defined, for example k8s ConfigMap
	hasWatchableResource bool
}

var consumer = sailor{
	sailorClient: &http.Client{},
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

// Initialize function initializes the sailor consumer with the given ResourceOption(s).
// If the ResourceOption(s) are empty, sailor doesn't consume anything.
// You can either provied ConnectionParam through `opts` or through ENV variables.
// If both of them are empty, sailor doesn't consume anything.
func Initialize(initOpts opts.InitOption) error {
	if len(initOpts.Resources) == 0 {
		return errors.New("no resources to manage, pass Resources inside opts")
	}

	if initOpts.Connection == nil {
		var conn = opts.ConnectionOption{}
		// we try getting all the necessary details from env

		if conn.Addr = os.Getenv(ENV_SAILOR_URL); conn.Addr == "" {
			return errors.New("cannot connect to sailor without address, either pass ENV_SAILOR_URL or set Connection in SailorOpts")
		}

		if conn.Namespace = os.Getenv(ENV_SAILOR_NS); conn.Namespace == "" {
			return errors.New("cannot connect to sailor without address, either pass ENV_SAILOR_NS or set Connection in SailorOpts")
		}

		if conn.App = os.Getenv(ENV_SAILOR_APP); conn.App == "" {
			return errors.New("cannot connect to sailor without address, either pass ENV_SAILOR_APP or set Connection in SailorOpts")
		}

		if conn.AccessKey = os.Getenv(ENV_SAILOR_ACCESS_KEY); conn.AccessKey == "" {
			return errors.New("cannot connect to sailor without address, either pass ENV_SAILOR_ACCESS_KEY or set Connection in SailorOpts")
		}

		if conn.SecretKey = os.Getenv(ENV_SAILOR_SECRET_KEY); conn.SecretKey == "" {
			return errors.New("cannot connect to sailor without address, either pass ENV_SAILOR_SECRET_KEY or set Connection in SailorOpts")
		}
	} else {
		consumer.opts = initOpts
	}

	configRef := make(map[string]any)
	consumer.configs.Store(&configRef)

	secretRef := make(map[string]string)
	consumer.secrets.Store(&secretRef)

	miscRef := make(map[string]string)
	consumer.misc.Store(&miscRef)

	consumer.watcher, _ = fsnotify.NewWatcher()

	// we will check what resources are required and how to manage them
	for _, res := range initOpts.Resources {
		switch res.Def.Kind {
		case opts.CONFIGS:
			if err := consumer.manageConfig(&res); err != nil {
				return err
			}
		case opts.SECRETS:
			if err := consumer.manageSecrets(&res); err != nil {
				return err
			}
		case opts.MISC:
			if err := consumer.manageMisc(&res); err != nil {
				return err
			}
		}
	}

	// this means that there are volume mounted resources which needs to be watched
	// for changes
	if consumer.hasWatchableResource {
		go consumer.watchForVolumeChanges()
	}

	return nil
}

// watchForVolumeChanges checks for all the paths mentioned in ResourceOption(s)
// which is of kind: Volume.
func (s *sailor) watchForVolumeChanges() {
	for {
		select {
		case event := <-consumer.watcher.Events:
			if event.Has(fsnotify.Write) {
				resourceName := path.Base(event.Name)
				if wi, ok := watcherFileNameResourceMap[resourceName]; ok {
					switch wi.kind {
					case opts.CONFIGS:
						configBytes, err := os.ReadFile(wi.path)
						if err != nil {
							log.Println("config has changed but unable to updated it due to: ", err.Error())
							continue
						}
						config, err := parseConfig(configBytes)
						if err != nil {
							log.Println("config has changed but unable to parse it due to: ", err.Error())
							continue
						}

						s.configs.Store(&config)
					case opts.SECRETS:
						secretBytes, err := os.ReadFile(wi.path)
						if err != nil {
							log.Println("secrets has changed but unable to updated it due to: ", err.Error())
							continue
						}

						var secret map[string]string
						if err := json.Unmarshal(secretBytes, &secret); err != nil {
							log.Println("secret has changed but unable to parse it due to: ", err.Error())
							continue
						}

						s.secrets.Store(&secret)
					case opts.MISC:
						miscBytes, err := os.ReadFile(wi.path)
						if err != nil {
							log.Println("misc has changed but unable to updated it due to: ", err.Error())
							continue
						}

						misc, err := parseMisc(miscBytes, wi.name)
						if err != nil {
							log.Println("misc has changed but unable to updated it due to: ", err.Error())
							continue
						}
						s.misc.Store(&misc)
					}
				}

			}
		case err := <-consumer.watcher.Errors:
			log.Println(err)
		}
	}
}

// parseConfig parses the config bytes as per the format saved by sailor. Since we need to support
// K8S deployment as well, we stringify the json under _content key.
func parseConfig(configBytes []byte) (map[string]any, error) {
	var content map[string]string
	err := json.Unmarshal(configBytes, &content)
	if err != nil {
		// :goto fallback
		return nil, err
	}

	var config map[string]any
	if err := json.Unmarshal([]byte(content["_content"]), &config); err != nil {
		// :goto fallback
		return nil, err
	}

	return config, nil
}

func parseMisc(miscBytes []byte, resourceName string) (map[string]string, error) {
	var content map[string]string
	err := json.Unmarshal(miscBytes, &content)
	if err != nil {
		// go to the fallback part
		return nil, err
	}

	oldMiscMap := consumer.misc.Load().(*map[string]string)
	var miscMap = map[string]string{
		resourceName: string(content["_content"]),
	}
	dst := maps.Clone(*oldMiscMap)
	maps.Copy(dst, miscMap)

	return dst, nil
}

// manageConfig manages the config defined inside Sailor for a given namespace and app
func (s *sailor) manageConfig(res *opts.ResourceOption) error {
	switch res.FetchDef.Fetch {
	case opts.VOLUME:
		// check if file is present in the path
		fileName := fmt.Sprintf("%s-config", s.opts.Connection.App)
		resourcePath := fmt.Sprintf("%s/%s", res.Def.Path, fileName)
		configBytes, err := os.ReadFile(resourcePath)
		if err == nil {
			config, err := parseConfig(configBytes)
			if err != nil {
				// :goto fallback
				break
			}
			s.configs.Store(&config)

			// add watcher details
			s.hasWatchableResource = true
			watcherFileNameResourceMap[fileName] = watcherInfo{opts.CONFIGS, resourcePath, ""}
			s.watcher.Add(resourcePath)

			return nil
		}

		if err := s.fetchFallback(res.Def.Kind, res.Def.Name); err != nil {
			return err
		}

		return nil
	case opts.PULL:
		// we will pull for the latest config with version
		url := fmt.Sprintf("%s/api/v1/resource/%s/%s/config",
			s.opts.Connection.Addr,
			s.opts.Connection.Namespace,
			s.opts.Connection.App,
		)

		resp, err := s.sailorClient.Get(url)
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

			var config map[string]any
			if err := json.Unmarshal(configBytes, &config); err != nil {
				// :goto fallback
				break
			}

			s.configs.Store(&config)

			// time to check if we want to pull the resource in background thread
			if !res.FetchDef.Once {
				go s.keepPullingResource(res)
			}

			return nil
		}

		if err := s.fetchFallback(res.Def.Kind, res.Def.Name); err != nil {
			return err
		}

		return nil
	}
	return nil
}

func (s *sailor) manageSecrets(res *opts.ResourceOption) error {
	switch res.FetchDef.Fetch {
	case opts.VOLUME:
		// check if file is present in the path
		fileName := fmt.Sprintf("%s-secret", s.opts.Connection.App)
		resourcePath := fmt.Sprintf("%s/%s", res.Def.Path, fileName)
		secretBytes, err := os.ReadFile(resourcePath)
		if err == nil {
			var secret map[string]string
			err = json.Unmarshal(secretBytes, &secret)
			if err != nil {
				// go to the fallback part
				break
			}

			s.secrets.Store(&secret)

			// add watcher details
			s.hasWatchableResource = true
			watcherFileNameResourceMap[fileName] = watcherInfo{opts.SECRETS, resourcePath, ""}
			s.watcher.Add(resourcePath)

			return nil
		}

		if err := s.fetchFallback(res.Def.Kind, res.Def.Name); err != nil {
			return err
		}

		return nil
	case opts.PULL:
		// we will pull for the latest config with version
		url := fmt.Sprintf("%s/api/v1/resource/%s/%s/secret",
			s.opts.Connection.Addr,
			s.opts.Connection.Namespace,
			s.opts.Connection.App,
		)

		resp, err := s.sailorClient.Get(url)
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

			var secret map[string]string
			if err := json.Unmarshal(secretBytes, &secret); err != nil {
				// goto fallback
				break
			}

			s.secrets.Store(&secret)

			// time to check if we want to pull the resource in background thread
			if !res.FetchDef.Once {
				go s.keepPullingResource(res)
			}

			return nil
		}

		if err := s.fetchFallback(res.Def.Kind, res.Def.Name); err != nil {
			return err
		}

		return nil
	}
	return nil
}

func (s *sailor) manageMisc(res *opts.ResourceOption) error {
	resourceName := fmt.Sprintf("%s-%s", res.Def.Name, "misc")
	switch res.FetchDef.Fetch {
	case opts.VOLUME:
		// check if file is present in the path
		fileName := fmt.Sprintf("%s-%s", s.opts.Connection.App, resourceName)
		resourcePath := fmt.Sprintf("%s/%s", res.Def.Path, fileName)
		miscBytes, err := os.ReadFile(resourcePath)
		if err == nil {
			miscMap, err := parseMisc(miscBytes, res.Def.Name)
			if err != nil {
				return err
			}
			s.misc.Store(&miscMap)

			// add watcher details
			s.hasWatchableResource = true
			watcherFileNameResourceMap[fileName] = watcherInfo{opts.MISC, resourcePath, res.Def.Name}
			s.watcher.Add(resourcePath)

			return nil
		}

		if err := s.fetchFallback(res.Def.Kind, res.Def.Name); err != nil {
			return err
		}

		return nil
	case opts.PULL:
		// we will pull for the latest config with version
		url := fmt.Sprintf("%s/api/v1/resource/%s/%s/misc/%s",
			s.opts.Connection.Addr,
			s.opts.Connection.Namespace,
			s.opts.Connection.App,
			res.Def.Name,
		)

		resp, err := s.sailorClient.Get(url)
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

			oldMiscMap := s.misc.Load().(*map[string]string)
			var miscMap = map[string]string{
				res.Def.Name: string(miscBytes),
			}
			if oldMiscMap == nil {
				s.misc.Store(&miscMap)
			} else {
				dst := maps.Clone(*oldMiscMap)
				maps.Copy(dst, miscMap)
				s.misc.Store(&dst)
			}

			// time to check if we want to pull the resource in background thread
			if !res.FetchDef.Once {
				go s.keepPullingResource(res)
			}

			return nil
		}

		if err := s.fetchFallback(res.Def.Kind, res.Def.Name); err != nil {
			return err
		}

		return nil
	}
	return nil
}

func (s *sailor) fetchFallback(forKind opts.ResourceKind, resName string) error {
	fallbackBaseURL := os.Getenv(ENV_SAILOR_FALLBACK_BASE_URL)
	if fallbackBaseURL != "" {
		url := fmt.Sprintf("%s/%s-%s.sailor.fall", fallbackBaseURL, s.opts.Connection.App, forKind)
		resp, err := s.sailorClient.Get(url)
		if err != nil {
			return err
		}

		resBytes, err := io.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err != nil {
			return err
		}

		if err = s.storeRawResource(resBytes, forKind, resName); err != nil {
			return err
		}

		return nil
	}

	return errors.New("cannot find config to serve, fallback fetch also failed")
}

func (s *sailor) keepPullingResource(res *opts.ResourceOption) {
	var url string
	switch res.Def.Kind {
	case opts.CONFIGS:
		url = fmt.Sprintf("%s/api/v1/resource/%s/%s/config",
			s.opts.Connection.Addr,
			s.opts.Connection.Namespace,
			s.opts.Connection.App,
		)
	case opts.SECRETS:
		url = fmt.Sprintf("%s/api/v1/resource/%s/%s/secret",
			s.opts.Connection.Addr,
			s.opts.Connection.Namespace,
			s.opts.Connection.App,
		)
	case opts.MISC:
		url = fmt.Sprintf("%s/api/v1/resource/%s/%s/misc/%s",
			s.opts.Connection.Addr,
			s.opts.Connection.Namespace,
			s.opts.Connection.App,
			res.Def.Name,
		)
	}

	resp, err := s.sailorClient.Get(url)
	if err == nil {
		if resp.StatusCode != http.StatusOK {
			time.Sleep(res.FetchDef.PullInterval)
			s.keepPullingResource(res)
			return
		}

		resBytes, err := io.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err != nil {
			time.Sleep(res.FetchDef.PullInterval)
			s.keepPullingResource(res)
			return
		}

		if err = s.storeRawResource(resBytes, res.Def.Kind, res.Def.Name); err != nil {
			time.Sleep(res.FetchDef.PullInterval)
			s.keepPullingResource(res)
			return
		}
	}

	time.Sleep(res.FetchDef.PullInterval)
	s.keepPullingResource(res)
}

func (s *sailor) storeRawResource(resBytes []byte, forKind opts.ResourceKind, resourceName string) error {
	switch forKind {
	case opts.CONFIGS:
		var config map[string]any
		if err := json.Unmarshal(resBytes, &config); err != nil {
			// TODO :: log here!
			return err
		}

		s.configs.Store(&config)
	case opts.SECRETS:
		var secret map[string]string
		if err := json.Unmarshal(resBytes, &secret); err != nil {
			// TODO :: log here!
			return err
		}

		s.secrets.Store(&secret)
	case opts.MISC:
		oldMiscMap := s.misc.Load().(*map[string]string)
		var miscMap = map[string]string{
			resourceName: string(resBytes),
		}
		if oldMiscMap == nil {
			s.misc.Store(&miscMap)
		} else {
			dst := maps.Clone(*oldMiscMap)
			maps.Copy(dst, miscMap)
			s.misc.Store(&dst)
		}
	}

	return nil
}
