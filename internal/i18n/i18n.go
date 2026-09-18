package i18n

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	i18ntemplate "github.com/nicksnyder/go-i18n/v2/i18n/template"
	"golang.org/x/text/language"
)

type localeKey struct{}

const contextLocaleKey = "goteams.i18n.locale"

const (
	LocaleSimplifiedChinese = "zh-CN"
	LocaleEnglishUS         = "en-US"

	headerAcceptLanguage  = "Accept-Language"
	headerContentLanguage = "Content-Language"
	headerLang            = "X-GoTeams-Lang"
	headerAppLang         = "lang"
)

// Middleware reads the request language once and stores the normalized locale
// in the Gin context. It also mirrors the chosen language into Content-Language
// so the browser and any client-side logging can inspect it directly.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		locale := resolveHeaders(c.GetHeader(headerLang), c.GetHeader(headerAppLang), c.GetHeader(headerAcceptLanguage))
		c.Set(contextLocaleKey, locale)
		c.Request = c.Request.WithContext(ContextWithLocale(c.Request.Context(), locale))
		c.Header(headerContentLanguage, locale)
		c.Next()
	}
}

// FromContext returns the locale attached by Middleware. Unknown or missing
// values always normalize to zh-CN so existing Chinese-only flows keep working.
func FromContext(c *gin.Context) string {
	if c == nil {
		return LocaleSimplifiedChinese
	}
	if v, ok := c.Get(contextLocaleKey); ok {
		if locale, ok := v.(string); ok && locale != "" {
			return Normalize(locale)
		}
	}
	return resolveHeaders(c.GetHeader(headerLang), c.GetHeader(headerAppLang), c.GetHeader(headerAcceptLanguage))
}

// Normalize maps a single client-provided language value to one of the supported
// locales. Accept-Language lists are handled by normalizeAcceptLanguage.
func Normalize(value string) string {
	if locale, ok := supportedLocale(value); ok {
		return locale
	}
	return LocaleSimplifiedChinese
}

func supportedLocale(value string) (string, bool) {
	tag, err := language.Parse(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
	if err != nil || tag == language.Und {
		return "", false
	}
	base, _ := tag.Base()
	switch base.String() {
	case "zh":
		return LocaleSimplifiedChinese, true
	case "en":
		return LocaleEnglishUS, true
	default:
		return "", false
	}
}

func resolveHeaders(values ...string) string {
	for index, value := range values {
		if strings.TrimSpace(value) != "" {
			if index == len(values)-1 {
				return normalizeAcceptLanguage(value)
			}
			return Normalize(value)
		}
	}
	return LocaleSimplifiedChinese
}

func normalizeAcceptLanguage(value string) string {
	tags, _, err := language.ParseAcceptLanguage(value)
	if err != nil {
		return LocaleSimplifiedChinese
	}
	for _, tag := range tags {
		if locale, ok := supportedLocale(tag.String()); ok {
			return locale
		}
	}
	return LocaleSimplifiedChinese
}

// T resolves the translation for a key in the current locale and falls back
// to zh-CN, then to the key itself when no entry exists.
func T(c *gin.Context, key string) string {
	return WithLocale(FromContext(c), key)
}

// WithLocale resolves a translation for an explicit locale.
func WithLocale(locale, key string) string {
	return FormatWithLocale(locale, key, nil)
}

// Params carries named template parameters. Count also selects a CLDR plural form.
type Params map[string]any

// Format renders a message using named parameters from the language file.
func Format(c *gin.Context, key string, params Params) string {
	return FormatWithLocale(FromContext(c), key, params)
}

func FormatWithLocale(locale, key string, params Params) string {
	return localize(bundle, Normalize(locale), key, params)
}

func localize(b *goi18n.Bundle, locale, key string, params Params) string {
	localizer := goi18n.NewLocalizer(b, locale)
	config := &goi18n.LocalizeConfig{
		MessageID:      key,
		TemplateData:   params,
		PluralCount:    params["Count"],
		TemplateParser: &i18ntemplate.TextParser{Option: "missingkey=error"},
	}
	message, err := localizer.Localize(config)
	if err == nil {
		return message
	}
	var missing *goi18n.MessageNotFoundErr
	if errors.As(err, &missing) {
		// go-i18n may return a Chinese fallback together with MessageNotFoundErr.
		if message != "" {
			return message
		}
		return key
	}
	// Never expose template errors, parameter values, or partially rendered text.
	fallback, _ := localizer.Localize(&goi18n.LocalizeConfig{MessageID: "common_server_error"})
	if fallback != "" {
		return fallback
	}
	return "common_server_error"
}

// WrapKey marks a system-generated value for deferred translation when it is persisted.
// User-provided text must never be wrapped.
func WrapKey(key string) string {
	if IsWrappedKey(key) {
		return key
	}
	return "${" + key + "}"
}

// IsWrappedKey reports whether value is a persisted system translation key.
func IsWrappedKey(value string) bool {
	return strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") && len(value) > 3
}

// ShowStored translates a wrapped system value and leaves user-provided text unchanged.
func ShowStored(locale, value string) string {
	if !IsWrappedKey(value) {
		return value
	}
	return WithLocale(locale, value[2:len(value)-1])
}

// ContextWithLocale returns a context that carries the resolved locale. It is
// useful for background work that wants to keep the request language.
func ContextWithLocale(ctx context.Context, locale string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, localeKey{}, Normalize(locale))
}

// LocaleFromContext extracts a locale from a standard context.
func LocaleFromContext(ctx context.Context) string {
	if ctx == nil {
		return LocaleSimplifiedChinese
	}
	if v, ok := ctx.Value(localeKey{}).(string); ok && v != "" {
		return Normalize(v)
	}
	return LocaleSimplifiedChinese
}

// IsSupported reports whether a locale is one of the supported values.
func IsSupported(locale string) bool {
	_, ok := supportedLocale(locale)
	return ok
}

// StatusText returns a translated fallback for generic HTTP status wording.
func StatusText(status int, locale string) string {
	switch status {
	case http.StatusBadRequest:
		return WithLocale(locale, "common_request_invalid")
	case http.StatusNotFound:
		return WithLocale(locale, "common_record_not_found")
	case http.StatusUnauthorized:
		return WithLocale(locale, "common_not_authenticated")
	default:
		return WithLocale(locale, "common_server_error")
	}
}

// Error aborts the current Gin chain and sends a localized error response with the common `{error, code}` shape.
func Error(c *gin.Context, status int, key string, code string) {
	c.Header(headerContentLanguage, FromContext(c))
	payload := gin.H{"error": T(c, key)}
	if code != "" {
		payload["code"] = code
	}
	c.AbortWithStatusJSON(status, payload)
}

// Message sends a localized success message in the common response shape.
func Message(c *gin.Context, status int, key string, params Params) {
	c.Header(headerContentLanguage, FromContext(c))
	c.JSON(status, gin.H{
		"message": Format(c, key, params),
	})
}
