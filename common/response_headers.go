package common

import (
	"net/http"
	"strings"
)

var responseHeaderBrands = strings.NewReplacer(
	"one-api", "nawei", "oneapi", "nawei", "one_api", "nawei",
	"new-api", "nawei", "newapi", "nawei", "new_api", "nawei",
)

// PublicResponseHeaderName translates gateway metadata names, never their values.
func PublicResponseHeaderName(name string) string {
	lower := strings.ToLower(name)
	renamed := responseHeaderBrands.Replace(lower)
	if renamed == lower {
		return name
	}
	return http.CanonicalHeaderKey(renamed)
}
