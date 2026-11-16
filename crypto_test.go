package sailor

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

const (
	katAccessKey = "server-access-key"
	katSecretKey = "server-secret-key"
	// SAILOR_CONSUMER.md section 6.3, known-answer vector 2.
	katRecord = `{"db_password":{"encrypted_secret":"lKaQNKO3FIX6i37ZNf+afEEEua9pVPOFAGKaLijlaW2pQklTFHtw278=","encrypted_dek":"Hf08s2skJt9HwGX2pptDX6GUCh/K97qQUhIgnFBk7FaqsvmWStMrlmclgPbYHphZ7m4AeT7w0jQ1zgQS"},"empty":{"encrypted_secret":"akkTYxk3ISelMKgWRgElDPkZopM70gE5Bi2hqw==","encrypted_dek":"UV6tqt7KQbHhINb4Kne1N9f9FH+DdFhtcLQQnHjWWDuul8lFr/JUXZFERCayB/X1QPscAcK0Q+rhRa5x"}}`
	// SAILOR_CONSUMER.md section 6.2, known-answer vector 1.
	katKEKHex = "9dbd6d9747e4182c853388a93ee76ad5decb1c284e4d9200256e0cbfc63a343d"
)

// C-25 and C-26
func TestC25_KnownAnswerDecryption(t *testing.T) {
	out, err := decryptResource([]byte(katRecord), katAccessKey, katSecretKey)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"db_password": "correct-horse", "empty": ""}
	if len(got) != len(want) || got["db_password"] != want["db_password"] || got["empty"] != "" {
		t.Fatalf("plaintext = %v, want %v", got, want)
	}
}

func TestC26_KEKKnownAnswer(t *testing.T) {
	kek, err := deriveKEK(katSecretKey, katAccessKey)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(kek) != katKEKHex {
		t.Fatalf("KEK = %x, want %s", kek, katKEKHex)
	}
}

// C-27: wrong secret key fails, and no partial map is returned.
func TestC27_WrongKeyFailsWholeResource(t *testing.T) {
	out, err := decryptResource([]byte(katRecord), katAccessKey, "wrong-secret")
	if !errors.Is(err, ErrDecryptFailed) {
		t.Fatalf("got %v, want DECRYPT_FAILED", err)
	}
	if out != nil {
		t.Fatalf("partial output returned: %s", out)
	}
}

// C-28: one flipped ciphertext byte is an authentication failure.
func TestC28_TamperedCiphertextFails(t *testing.T) {
	var recs map[string]secretRecord
	if err := json.Unmarshal([]byte(katRecord), &recs); err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(recs["db_password"].EncryptedSecret)
	raw[len(raw)-1] ^= 0x01
	recs["db_password"] = secretRecord{EncryptedSecret: base64.StdEncoding.EncodeToString(raw), EncryptedDEK: recs["db_password"].EncryptedDEK}
	body, _ := json.Marshal(recs)
	if _, err := decryptResource(body, katAccessKey, katSecretKey); !errors.Is(err, ErrDecryptFailed) {
		t.Fatalf("got %v, want DECRYPT_FAILED", err)
	}
}

// C-29: a short encrypted_dek fails cleanly rather than panicking (CONS-CRYPT-8).
func TestC29_ShortDEKFailsWithoutPanic(t *testing.T) {
	short := base64.StdEncoding.EncodeToString(make([]byte, 11))
	body := []byte(`{"k":{"encrypted_secret":"AAAA","encrypted_dek":"` + short + `"}}`)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	if _, err := decryptResource(body, katAccessKey, katSecretKey); !errors.Is(err, ErrDecryptFailed) {
		t.Fatalf("got %v, want DECRYPT_FAILED", err)
	}
}

// C-30: URL-safe base64 is rejected.
func TestC30_URLSafeBase64Rejected(t *testing.T) {
	var recs map[string]secretRecord
	_ = json.Unmarshal([]byte(katRecord), &recs)
	rec := recs["db_password"]
	rec.EncryptedDEK = strings.NewReplacer("+", "-", "/", "_").Replace(rec.EncryptedDEK)
	recs["db_password"] = rec
	body, _ := json.Marshal(recs)
	if _, err := decryptResource(body, katAccessKey, katSecretKey); err == nil {
		t.Fatal("URL-safe base64 was accepted")
	}
}

// C-31: an empty plaintext decodes to 28 bytes and yields "".
func TestC31_EmptyPlaintext(t *testing.T) {
	body := testEncryptedBody(t, katAccessKey, katSecretKey, map[string]string{"empty": ""})
	out, err := decryptResource(body, katAccessKey, katSecretKey)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"empty":""}` {
		t.Fatalf("out = %s", out)
	}
}

// C-33: non-ASCII UTF-8 round-trips unchanged.
func TestC33_UnicodeRoundTrip(t *testing.T) {
	const value = "héllo wörld 🔐"
	body := testEncryptedBody(t, katAccessKey, katSecretKey, map[string]string{"k": value})
	out, err := decryptResource(body, katAccessKey, katSecretKey)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	_ = json.Unmarshal(out, &got)
	if got["k"] != value {
		t.Fatalf("got %q", got["k"])
	}
}

// C-32: a header that disagrees with the declaration is logged and the declaration wins.
// The header is observed at the HTTP layer, so the decision is covered in lifecycle tests.

// Language handling (CONS-RES-5, CONS-RES-6, CONS-RES-7).
func TestLangNormalisationAndDecoders(t *testing.T) {
	if normalizeLang("  YML ") != "yaml" || normalizeLang("") != "json" {
		t.Fatal("normalizeLang mapping is wrong")
	}
	set := newDecoderSet(map[string]opts.DecodeFunc{"yaml": func(b []byte, v any) error { return nil }})
	if _, err := set.lookup("yml"); err != nil {
		t.Fatalf("yml should resolve to registered yaml decoder: %v", err)
	}
	if _, err := set.lookup("toml"); !errors.Is(err, ErrUnsupportedLang) {
		t.Fatalf("toml lookup = %v, want UNSUPPORTED_LANG", err)
	}
}

func TestDecodersArePerInstance(t *testing.T) {
	a := newDecoderSet(map[string]opts.DecodeFunc{"x": func([]byte, any) error { return nil }})
	b := newDecoderSet(nil)
	if _, err := a.lookup("x"); err != nil {
		t.Fatal("instance a lost its decoder")
	}
	if _, err := b.lookup("x"); err == nil {
		t.Fatal("decoder leaked into instance b")
	}
}

// Produced by the backend's own vault package (sailor/pkg/vault) with the
// same test keys, so this checks the backend's encrypt path against this decoder.
const vaultRecord = `{"db_password":{"encrypted_secret":"fHJ1Wu9sBsZ5wiIiaQqnqapN/HadHe7dxU8pqjZ4UHKkKrgU1sKQvfwwwwQVqg==","encrypted_dek":"REE0Ymp5OV6yg2AMJRrJGVws7QjKH5SyJoo+/wsDzy9agNgIZJ00pU9z+65Hlj3hjJNAOqmHmkINX3Lc"},"empty":{"encrypted_secret":"ocOhDiRRKSNVyoAXqkTAp5WotIGnE4Yc+lX91A==","encrypted_dek":"vzjKYtWFUl+I6VzGaIFgaUMsUTOMsHC+dqrMxW9plUsPhFWK0C9gsb+6KPFaAMJ3/XGxWZ4GMHbSVCA1"}}`

func TestBackendEncryptedRecordDecrypts(t *testing.T) {
	out, err := decryptResource([]byte(vaultRecord), katAccessKey, katSecretKey)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["db_password"] != "héllo wörld 🔐" || got["empty"] != "" {
		t.Fatalf("plaintext = %v", got)
	}
}
