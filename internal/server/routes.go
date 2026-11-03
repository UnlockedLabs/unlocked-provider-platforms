package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/UnlockedLabs/unlocked-contracts/identity"
	"github.com/UnlockedLabs/unlocked-contracts/providerplatforms"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/module"
)

// Routes mounts the module's routes. Core is the public edge and checks who
// may call each route, so a route is guarded only by the service token, and
// the caller's claims come from core's header.
func Routes(token string, routes []module.Route) Mount {
	return func(mux *http.ServeMux, prefix string) {
		for _, rt := range routes {
			method, path, _ := strings.Cut(rt.Pattern, " ")
			mux.Handle(method+" "+prefix+path, requireToken(token, withClaims(rt.Handler)))
		}
	}
}

// Internal mounts the contracts' Handler, which answers core's typed calls
// under /internal/v1/provider-platforms/. The Handler checks the token itself.
// It sits at the root, not under PATH_PREFIX: ingress never routes it, core
// reaches it at the service's own address.
func Internal(token string, pp providerplatforms.ProviderPlatforms) Mount {
	return func(mux *http.ServeMux, _ string) {
		mux.Handle("/internal/v1/provider-platforms/", providerplatforms.NewHandler(pp, token))
	}
}

// requireToken serves next only to requests that carry token as a bearer
// token, as the contracts' clients send it. An empty token rejects every
// request, so a service started without one fails closed.
func requireToken(token string, next http.Handler) http.Handler {
	want := sha256.Sum256([]byte("Bearer " + token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if token == "" || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			http.Error(w, "invalid service token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withClaims puts the caller's claims from identity.ClaimsHeader on the request's
// context, and answers 401 when there are none.
func withClaims(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := identity.ClaimsFromRequest(r)
		if !ok {
			http.Error(w, "missing caller claims", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(identity.NewContext(r.Context(), c)))
	})
}
