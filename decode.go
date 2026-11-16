package sailor

import (
	"encoding/json"
	"strings"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

// normalizeLang implements CONS-RES-5.
func normalizeLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return "json"
	}
	if lang == "yml" {
		return "yaml"
	}
	return lang
}

func jsonDecode(raw []byte, target any) error { return json.Unmarshal(raw, target) }

// decoderSet is the per-instance decoder registry (CONS-RES-6, CONS-RES-8).
type decoderSet map[string]opts.DecodeFunc

func newDecoderSet(extra map[string]opts.DecodeFunc) decoderSet {
	set := decoderSet{"json": jsonDecode}
	for lang, fn := range extra {
		set[normalizeLang(lang)] = fn
	}
	return set
}

// lookup returns the decoder for lang, or ErrUnsupportedLang (CONS-RES-7).
func (s decoderSet) lookup(lang string) (opts.DecodeFunc, error) {
	fn, ok := s[normalizeLang(lang)]
	if !ok {
		return nil, errf(CodeUnsupportedLang, "no decoder registered for lang %q", normalizeLang(lang))
	}
	return fn, nil
}
