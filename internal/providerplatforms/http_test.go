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
