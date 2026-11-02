package providerplatforms

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/UnlockedLabs/unlocked-contracts/coreapi"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type routeRecorder struct{ routes []module.Route }

func (r *routeRecorder) Handle(rt module.Route) { r.routes = append(r.routes, rt) }

// Core proxies these exact patterns, so the set is pinned.
func TestRegisterRoutes(t *testing.T) {
	rec := &routeRecorder{}
	Module{}.Register(rec, module.Deps{DB: newTestDB(t)})

	var patterns []string
	for _, rt := range rec.routes {
		require.NotNil(t, rt.Handler, rt.Pattern)
		patterns = append(patterns, rt.Pattern)
	}
	assert.Len(t, patterns, 30)
	assert.ElementsMatch(t, []string{
		"GET /api/provider-platforms",
		"GET /api/provider-platforms/{id}",
		"POST /api/provider-platforms",
		"GET /api/provider-platforms/callback",
		"GET /api/provider-platforms/{id}/refresh",
		"PATCH /api/provider-platforms/{id}",
		"DELETE /api/provider-platforms/{id}",
		"POST /api/provider-platforms/{id}/map-user/{user_id}",
		"POST /api/provider-platforms/{id}/users/import",
		"POST /api/provider-platforms/{id}/create-user/{user_id}",
		"GET /api/users/{id}/logins",
		"POST /api/users/{id}/logins",
		"POST /api/provider-platforms/{id}/user-accounts/{user_id}",
		"DELETE /api/users/{userId}/logins/{providerId}",
		"GET /api/provider-platforms/{id}/mapped-users",
		"GET /api/actions/provider-platforms/{id}/get-users",
		"POST /api/actions/provider-platforms/{id}/import-users",
		"GET /api/actions/provider-platforms/{id}/match-users",
		"POST /api/actions/provider-platforms/{id}/apply-matches",
		"GET /api/oidc/clients",
		"POST /api/oidc/clients",
		"GET /api/oidc/clients/{id}",
		"POST /api/oidc/clients/{id}/recreate",
		"GET /api/courses",
		"GET /api/courses/{id}",
		"GET /api/courses/{id}/activity",
		"GET /api/users/{id}/outcomes",
		"GET /api/users/{id}/daily-activity",
		"GET /api/users/{id}/catalog",
		"GET /api/users/{id}/courses",
	}, patterns)
}

// A user core doesn't have answers as before the move, when core's Users
// returned gorm.ErrRecordNotFound.
func TestMissingCoreUserIsBadRequest(t *testing.T) {
	err := newNotFoundDBError(fmt.Errorf("get user 9: %w", coreapi.ErrUserNotFound), "users")
	assert.Equal(t, http.StatusBadRequest, err.Status)
}
