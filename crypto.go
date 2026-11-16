package sailor

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
)

// SAILOR_CONSUMER.md section 6. Sizes are fixed by the wire format.
const (
	nonceSize = 12
	tagSize   = 16
	dekSize   = 32
	// encryptedDEKSize is nonce || wrapped DEK || tag: 12 + 32 + 16.
	encryptedDEKSize = nonceSize + dekSize + tagSize
	kekSize          = 32
)

type secretRecord struct {
	EncryptedSecret string `json:"encrypted_secret"`
	EncryptedDEK    string `json:"encrypted_dek"`
}

// deriveKEK implements CONS-CRYPT-3: HKDF-SHA256, IKM = SecretKey, salt = AccessKey, empty info.
func deriveKEK(secretKey, accessKey string) ([]byte, error) {
	return hkdf.Key(sha256.New, []byte(secretKey), []byte(accessKey), "", kekSize)
}

// gcmOpen decodes the nonce || ciphertext || tag layout (CONS-CRYPT-5). The
// length is checked before any slice (CONS-CRYPT-8).
func gcmOpen(data, key []byte) ([]byte, error) {
	if len(data) < nonceSize+tagSize {
		return nil, errors.New("ciphertext shorter than nonce and tag")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
}

// decryptResource turns an encrypted resource body into the flat JSON object
// of plaintext strings (CONS-CRYPT-10). Any failing key fails the whole resource
// (CONS-CRYPT-11). Errors name the key and never carry a value.
func decryptResource(body []byte, accessKey, secretKey string) ([]byte, error) {
	var records map[string]secretRecord
	if err := json.Unmarshal(body, &records); err != nil {
		return nil, wrapf(CodeDecodeFailed, err, "encrypted resource is not a map of records")
	}
	kek, err := deriveKEK(secretKey, accessKey)
	if err != nil {
		return nil, wrapf(CodeDecryptFailed, err, "deriving key-encryption key")
	}

	keys := make([]string, 0, len(records))
	for k := range records {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make(map[string]string, len(records))
	for _, k := range keys {
		rec := records[k]
		wrapped, err := base64.StdEncoding.DecodeString(rec.EncryptedDEK)
		if err != nil {
			return nil, wrapf(CodeDecryptFailed, err, "key %q: encrypted_dek is not standard base64", k)
		}
		if len(wrapped) != encryptedDEKSize {
			return nil, errf(CodeDecryptFailed, "key %q: encrypted_dek is %d bytes, want %d", k, len(wrapped), encryptedDEKSize)
		}
		dek, err := gcmOpen(wrapped, kek)
		if err != nil {
			return nil, wrapf(CodeDecryptFailed, err, "key %q: unwrapping data key", k)
		}
		if len(dek) != dekSize {
			return nil, errf(CodeDecryptFailed, "key %q: data key is %d bytes, want %d", k, len(dek), dekSize)
		}

		secret, err := base64.StdEncoding.DecodeString(rec.EncryptedSecret)
		if err != nil {
			return nil, wrapf(CodeDecryptFailed, err, "key %q: encrypted_secret is not standard base64", k)
		}
		plain, err := gcmOpen(secret, dek)
		if err != nil {
			return nil, wrapf(CodeDecryptFailed, err, "key %q: decrypting value", k)
		}
		out[k] = string(plain)
	}
	return json.Marshal(out)
}
