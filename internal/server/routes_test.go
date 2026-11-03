package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/UnlockedLabs/unlocked-contracts/identity"
	"github.com/UnlockedLabs/unlocked-contracts/providerplatforms"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/module"
)

const testToken = "s3cret"

func authed(method, path string, claims *identity.Claims) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("Authorization", "Bearer "+testToken)
	if claims != nil {
		b, _ := json.Marshal(claims)
		r.Header.Set(ClaimsHeader, string(b))
	}
	return r
}

func TestRequireToken(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	tests := []struct {
		name, configured, sent string
		want                   int
	}{
		{"right token", testToken, "Bearer " + testToken, http.StatusNoContent},
		{"wrong token", testToken, "Bearer nope", http.StatusUnauthorized},
		{"token without the Bearer scheme", testToken, testToken, http.StatusUnauthorized},
		{"missing token", testToken, "", http.StatusUnauthorized},
		{"empty configured token rejects everything", "", "Bearer ", http.StatusUnauthorized},
		{"lowercase scheme", testToken, "bearer " + testToken, http.StatusUnauthorized},
		{"trailing space", testToken, "Bearer " + testToken + " ", http.StatusUnauthorized},
		{"leading space", testToken, " Bearer " + testToken, http.StatusUnauthorized},
		{"token as a prefix of the sent value", testToken, "Bearer " + testToken + "x", http.StatusUnauthorized},
		{"sent value as a prefix of the token", testToken, "Bearer " + testToken[:3], http.StatusUnauthorized},
		{"basic scheme", testToken, "Basic " + testToken, http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.sent != "" {
				r.Header.Set("Authorization", tt.sent)
			}
			w := httptest.NewRecorder()
			requireToken(tt.configured, next).ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}

func TestRequireTokenIgnoresOtherPlaces(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	tests := map[string]func(r *http.Request){
		"token in the query string":      func(r *http.Request) { r.URL.RawQuery = "token=" + testToken },
		"the old X-Service-Token header": func(r *http.Request) { r.Header.Set("X-Service-Token", testToken) },
		"right token as a second header": func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer nope")
			r.Header.Add("Authorization", "Bearer "+testToken)
		},
	}
	for name, set := range tests {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			set(r)
			w := httptest.NewRecorder()
			requireToken(testToken, next).ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", w.Code)
			}
		})
	}
}

func TestRoutes(t *testing.T) {
	var got *identity.Claims
	rt := module.Route{Pattern: "GET /api/courses/{id}", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = identity.FromContext(r.Context())
		_, _ = w.Write([]byte(r.PathValue("id")))
	})}
	h := newHandler(t, "/pp", Version{}, Routes(testToken, []module.Route{rt}))

	t.Run("serves a route under the prefix with the caller's claims", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, authed(http.MethodGet, "/pp/api/courses/7", &identity.Claims{UserID: 3, FacilityID: 2}))
		if w.Code != http.StatusOK || w.Body.String() != "7" {
			t.Fatalf("got %d %q, want 200 \"7\"", w.Code, w.Body.String())
		}
		if got == nil || got.UserID != 3 || got.FacilityID != 2 {
			t.Fatalf("claims = %+v, want user 3 at facility 2", got)
		}
	})
	t.Run("no token is a 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/pp/api/courses/7", nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})
	t.Run("a token without the caller's claims is a 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, authed(http.MethodGet, "/pp/api/courses/7", nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})
	t.Run("claims that aren't JSON are a 401", func(t *testing.T) {
		r := authed(http.MethodGet, "/pp/api/courses/7", nil)
		r.Header.Set(ClaimsHeader, "{")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})
	t.Run("outside the prefix is a 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, authed(http.MethodGet, "/api/courses/7", &identity.Claims{}))
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})
}

type fakePP struct {
	providerplatforms.ProviderPlatforms
}

func (fakePP) ActiveProviders(context.Context) ([]providerplatforms.Provider, error) {
	return []providerplatforms.Provider{{ID: 4, Name: "canvas"}}, nil
}

func TestInternal(t *testing.T) {
	srv := httptest.NewServer(newHandler(t, "/pp", Version{}, Internal(testToken, fakePP{})))
	t.Cleanup(srv.Close)

	t.Run("the contracts client reaches the handlers under the prefix", func(t *testing.T) {
		got, err := providerplatforms.NewClient(srv.URL+"/pp", testToken, nil).ActiveProviders(context.Background())
		if err != nil || len(got) != 1 || got[0].ID != 4 {
			t.Fatalf("ActiveProviders() = %+v, %v, want provider 4", got, err)
		}
	})
	t.Run("a wrong token is rejected", func(t *testing.T) {
		_, err := providerplatforms.NewClient(srv.URL+"/pp", "wrong", nil).ActiveProviders(context.Background())
		if err == nil {
			t.Fatal("ActiveProviders() error = nil, want one")
		}
	})
}
