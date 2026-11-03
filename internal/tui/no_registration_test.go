package tui

import (
	"strings"
	"testing"
)

func TestAllLanguagesRenderWithoutRegistrationPrompts(t *testing.T) {
	for _, lang := range []string{"ar", "bn", "de", "en", "es", "fr", "hi", "id", "ja", "ko", "pt", "ru", "ur", "vi", "zh"} {
		t.Run(lang, func(t *testing.T) {
			localized, fallback := loadDict(lang)
			for _, catalog := range []dict{localized, fallback} {
				for key := range catalog {
					if strings.HasPrefix(key, "lic.") || strings.HasPrefix(key, "mark.") || strings.HasPrefix(key, "sponsor.") || key == "sl.locked" || key == "emb.locked" {
						t.Fatalf("obsolete registration or advertising message: %s", key)
					}
				}
				if catalog["login.hint"] == "" || catalog["acct.rule"] == "" {
					t.Fatal("local-account security translations missing")
				}
			}
			a := newApp(Options{Lang: lang})
			frame := strings.ToLower(strings.Join(a.frame(), "\n"))
			if !strings.Contains(frame, "vyos-sflow") {
				t.Fatal("product header missing")
			}
			for _, forbidden := range []string{"evaluation copy", "not registered", "sponsor", "lic.install", "mark.text"} {
				if strings.Contains(frame, forbidden) {
					t.Fatalf("unexpected prompt in terminal: %s", forbidden)
				}
			}
		})
	}
}
