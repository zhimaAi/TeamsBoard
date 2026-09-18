package i18n

import (
	"embed"
	"fmt"
	"io/fs"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/zh-CN.json locales/en-US.json
var localeFiles embed.FS

// The bundle is initialized once and never modified while requests use it.
var bundle = mustLoadBundle(localeFiles)

func mustLoadBundle(files fs.FS) *goi18n.Bundle {
	b, err := loadBundle(files)
	if err != nil {
		panic(err)
	}
	return b
}

func loadBundle(files fs.FS) (*goi18n.Bundle, error) {
	b := goi18n.NewBundle(language.MustParse(LocaleSimplifiedChinese))
	catalogs := make(map[string]map[string]*goi18n.Message, 2)
	for _, locale := range []string{LocaleSimplifiedChinese, LocaleEnglishUS} {
		name := "locales/" + locale + ".json"
		data, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, fmt.Errorf("i18n: read %s: %w", name, err)
		}
		entries, err := validateMessageFile(data)
		if err != nil {
			return nil, fmt.Errorf("i18n: validate %s: %w", name, err)
		}
		if _, err := b.ParseMessageFileBytes(data, name); err != nil {
			return nil, fmt.Errorf("i18n: load %s: %w", name, err)
		}
		catalogs[locale] = entries
	}
	if err := validateCatalogParity(catalogs[LocaleSimplifiedChinese], catalogs[LocaleEnglishUS]); err != nil {
		return nil, err
	}
	return b, nil
}
