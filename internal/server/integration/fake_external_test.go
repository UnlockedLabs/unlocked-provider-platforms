package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// fakeExternal stands in for everything Provider Platforms calls over HTTP:
// its worker's user lists, Kratos, Hydra and Kolibri. IDs it hands out count up,
// so the golden files are stable.
type fakeExternal struct {
	*httptest.Server
	mu         sync.Mutex
	identities int
	clients    int
}

// providerUser is a user's account at a provider, as the worker sends it.
type providerUser struct {
	Username         string `json:"username"`
	NameFirst        string `json:"name_first"`
	NameLast         string `json:"name_last"`
	Email            string `json:"email"`
	ExternalUserID   string `json:"external_user_id"`
	ExternalUsername string `json:"external_username"`
}

// The users the worker returns for the Kolibri provider. /api/users
// is the unmapped ones; /api/users/all adds the one mapped to jdoe.
var (
	unmappedProviderUsers = []providerUser{
		{Username: "mary.major", NameFirst: "Mary", NameLast: "Major", Email: "mary@example.com", ExternalUserID: "k-2002", ExternalUsername: "mary.major"},
		{Username: "jane.smith", NameFirst: "Jane", NameLast: "Smith", Email: "jane@example.com", ExternalUserID: "k-2003", ExternalUsername: "jane.smith"},
	}
	allProviderUsers = append([]providerUser{
		{Username: "john.doe", NameFirst: "John", NameLast: "Doe", Email: "john@example.com", ExternalUserID: "k-1001", ExternalUsername: "john.doe"},
	}, unmappedProviderUsers...)
)

func newFakeExternal(t *testing.T) *fakeExternal {
	t.Helper()
	f := &fakeExternal{}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	identity := func(id string) map[string]any {
		return map[string]any{"id": id, "schema_id": "default", "schema_url": "http://kratos.test/schema", "traits": map[string]any{}}
	}

	// The worker's user lists
	mux.HandleFunc("GET /api/users", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, unmappedProviderUsers)
	})
	mux.HandleFunc("GET /api/users/all", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, allProviderUsers)
	})

	// Kratos
	mux.HandleFunc("POST /admin/identities", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.identities++
		id := fmt.Sprintf("kratos-%d", f.identities)
		f.mu.Unlock()
		writeJSON(w, http.StatusCreated, identity(id))
	})
	mux.HandleFunc("GET /admin/identities/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, identity(r.PathValue("id")))
	})
	mux.HandleFunc("PUT /admin/identities/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, identity(r.PathValue("id")))
	})

	// Hydra
	mux.HandleFunc("POST /admin/clients", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.clients++
		n := f.clients
		f.mu.Unlock()
		writeJSON(w, http.StatusCreated, map[string]any{
			"client_id":     fmt.Sprintf("client-%d", n),
			"client_name":   body["client_name"],
			"client_secret": fmt.Sprintf("secret-%d", n),
			"scope":         body["scope"],
		})
	})
	mux.HandleFunc("DELETE /admin/clients/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Kolibri
	mux.HandleFunc("POST /api/public/signup/", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		writeJSON(w, http.StatusCreated, map[string]any{"id": fmt.Sprintf("kolibri-%v", body["username"])})
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected call to fake external service: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}
