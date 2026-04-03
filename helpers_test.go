package sailor

import (
	"github.com/sailorhq/sailor/pkg/vault"
	"testing"
	"time"
	"github.com/sailorhq/sailor-go/pkg/opts"
)

func EncryptSecretForTest(ak, sk string, secrets map[string]string) (map[string]vault.SecretRecord, error) {
	kek, err := vault.DeriveKEK(sk, []byte(ak))
	if err != nil {
		return nil, err
	}

	encSecrets := make(map[string]vault.SecretRecord)
	for k, v := range secrets {
		dek, err := vault.GenerateDEK()
		if err != nil {
			return nil, err
		}

		encSecret, _, err := vault.EncryptWithDEK(v, dek)
		if err != nil {
			return nil, err
		}

		encDek, err := vault.EncryptDEK(dek, kek)
		if err != nil {
			return nil, err
		}

		encSecrets[k] = vault.SecretRecord{
			EncryptedDEK:    encDek,
			EncryptedSecret: encSecret,
		}
	}
	return encSecrets, nil
}

func TestVolumeSecretsCorrectData(t *testing.T) {
	type DummySecret struct {
		Password string `json:"password"`
	}

	ak := "ak"
	sk := "sk"
	password := "supersecret"

	secretsMap := map[string]string{
		"password": password,
	}

	encSecrets, err := EncryptSecretForTest(ak, sk, secretsMap)
	if err != nil {
		t.Fatal(err)
	}

	createTestFile(encSecrets, "_secret")
	defer removeTestFile("_secret")

	consumer, err := NewConsumer[any, DummySecret](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.SECRETS,
					Path: testFolder,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
			},
		},
		Connection: &opts.ConnectionOption{
			Addr:          "http://localhost:7766",
			Namespace:     "test",
			App:           "test",
			AccessKey:     ak,
			SecretKey:     sk,
			SocketTimeout: time.Second * 5,
		},
	})

	if err != nil {
		t.Error(err)
		return
	}

	if err = consumer.Start(); err != nil {
		t.Error(err)
		return
	}

	secret, err := consumer.GetSecret()
	if err != nil {
		t.Error(err)
		return
	}

	if secret.Password != password {
		t.Errorf("required %s got %s", password, secret.Password)
	}
}

func TestVolumeMiscCorrectData(t *testing.T) {
	miscContent := "misc data content"
	resourceName := "my-misc"

	createTestFile(miscContent, "_" + resourceName)
	defer removeTestFile("_" + resourceName)

	consumer, err := NewConsumer[any, any](opts.InitOption{
		Resources: []opts.ResourceOption{
			{
				Def: opts.ResourceDefinition{
					Kind: opts.MISC,
					Name: resourceName,
					Path: testFolder,
				},
				FetchDef: opts.FetchDefinition{
					Fetch: opts.VOLUME,
				},
			},
		},
		Connection: &opts.ConnectionOption{
			Addr:          "http://localhost:7766",
			Namespace:     "test",
			App:           "test",
			AccessKey:     "ak",
			SecretKey:     "sk",
			SocketTimeout: time.Second * 5,
		},
	})

	if err != nil {
		t.Error(err)
		return
	}

	if err = consumer.Start(); err != nil {
		t.Error(err)
		return
	}

	misc, err := consumer.GetMisc(resourceName)
	if err != nil {
		t.Error(err)
		return
	}

	if string(misc) != "\"misc data content\"" {
		t.Errorf("expected %s got %s", "\"misc data content\"", string(misc))
	}
}
