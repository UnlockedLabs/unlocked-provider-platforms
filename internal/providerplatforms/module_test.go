package providerplatforms

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/UnlockedLabs/unlocked-contracts/coreapi"
	"github.com/UnlockedLabs/unlocked-contracts/providerplatforms"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type routeRecorder struct{ routes []module.Route }

func (r *routeRecorder) Handle(rt module.Route) { r.routes = append(r.routes, rt) }

// Core's proxy enforces access from the contracts' route table, so the service
// must serve exactly those patterns.
func TestRegisterRoutesMatchContracts(t *testing.T) {
	rec := &routeRecorder{}
	Module{}.Register(rec, module.Deps{DB: newTestDB(t)})

	var got, want []string
	for _, rt := range rec.routes {
		require.NotNil(t, rt.Handler, rt.Pattern)
		got = append(got, rt.Pattern)
	}
	for _, rt := range providerplatforms.Routes {
		want = append(want, rt.Pattern)
	}
	assert.ElementsMatch(t, want, got)
}

// A user core doesn't have answers as before the move, when core's Users
// returned gorm.ErrRecordNotFound.
func TestMissingCoreUserIsBadRequest(t *testing.T) {
	err := newNotFoundDBError(fmt.Errorf("get user 9: %w", coreapi.ErrUserNotFound), "users")
	assert.Equal(t, http.StatusBadRequest, err.Status)
}
