package sailor

import (
	"testing"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

func TestDefaults(t *testing.T) {
	configMap := ConfigMapDefault()
	if configMap.Def.Kind != opts.CONFIGS || configMap.Def.Path != volume_mount_path || configMap.FetchDef.Fetch != opts.VOLUME || !configMap.FallbackEnabled {
		t.Errorf("ConfigMapDefault failed: %+v", configMap)
	}

	secrets := SecretsDefault()
	if secrets.Def.Kind != opts.SECRETS || secrets.Def.Path != volume_mount_path || secrets.FetchDef.Fetch != opts.VOLUME || !secrets.FallbackEnabled {
		t.Errorf("SecretsDefault failed: %+v", secrets)
	}

	miscOnce := MiscOnceDefault("test-misc")
	if miscOnce.Def.Kind != opts.MISC || miscOnce.Def.Name != "test-misc" || miscOnce.Def.Path != volume_mount_path || miscOnce.FetchDef.Fetch != opts.PULL || !miscOnce.FetchDef.Once || !miscOnce.FallbackEnabled {
		t.Errorf("MiscOnceDefault failed: %+v", miscOnce)
	}

	configPull := ConfigPullDefault()
	if configPull.Def.Kind != opts.CONFIGS || configPull.FetchDef.Fetch != opts.PULL || configPull.FetchDef.PullInterval != 10*time.Second || !configPull.FallbackEnabled {
		t.Errorf("ConfigPullDefault failed: %+v", configPull)
	}

	secretsPull := SecretsPullDefault()
	if secretsPull.Def.Kind != opts.SECRETS || secretsPull.FetchDef.Fetch != opts.PULL || secretsPull.FetchDef.PullInterval != 10*time.Second || !secretsPull.FallbackEnabled {
		t.Errorf("SecretsPullDefault failed: %+v", secretsPull)
	}

	miscPull := MiscPullDefault("test-misc-pull")
	if miscPull.Def.Kind != opts.MISC || miscPull.Def.Name != "test-misc-pull" || miscPull.FetchDef.Fetch != opts.PULL || miscPull.FetchDef.PullInterval != 10*time.Second || !miscPull.FallbackEnabled {
		t.Errorf("MiscPullDefault failed: %+v", miscPull)
	}
}
