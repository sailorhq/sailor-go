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
