package providerplatforms

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A failed request's error line names the module, method and path.
func TestHandleErrorLogFields(t *testing.T) {
	oldHooks := logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
	defer logrus.StandardLogger().ReplaceHooks(oldHooks)
	hook := logtest.NewGlobal()

	r := httptest.NewRequest(http.MethodPut, "/api/things/1", nil)
	w := httptest.NewRecorder()
	handle(func(http.ResponseWriter, *http.Request, sLog) error {
		return errors.New("boom")
	}).ServeHTTP(w, r)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	entry := hook.LastEntry()
	require.NotNil(t, entry)
	assert.Equal(t, logrus.ErrorLevel, entry.Level)
	assert.Equal(t, "providerplatforms", entry.Data["module"])
	assert.Equal(t, http.MethodPut, entry.Data["method"])
	assert.Equal(t, "/api/things/1", entry.Data["path"])
}

func TestOrderClauseOnlyAllowsAscOrDesc(t *testing.T) {
	q := queryContext{OrderBy: "created_at", Order: "desc; drop table users"}
	assert.Equal(t, "created_at desc", q.OrderClause("fallback"))
	q.Order = "asc"
	assert.Equal(t, "created_at asc", q.OrderClause("fallback"))
	assert.Equal(t, "created_at", queryContext{OrderBy: "created_at"}.OrderClause("fallback"))
	assert.Equal(t, "fallback", queryContext{}.OrderClause("fallback"))
}
