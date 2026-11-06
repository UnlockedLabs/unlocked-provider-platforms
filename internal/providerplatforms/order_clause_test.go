package providerplatforms

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOrderClauseFallsBack(t *testing.T) {
	cases := []struct{ name, orderBy, order, want string }{
		{"asc", "created_at", "asc", "created_at asc"},
		{"desc", "created_at", "desc", "created_at desc"},
		{"upper case direction", "created_at", "DESC", "created_at desc"},
		{"empty direction defaults to asc", "created_at", "", "created_at asc"},
		{"subselect in direction", "created_at", "desc,(select 1/0)", "fallback"},
		{"statement in direction", "created_at", "desc; drop table users", "fallback"},
		{"unknown direction", "created_at", "sideways", "fallback"},
		{"whitespace direction", "created_at", " ", "fallback"},
		{"hostile column without direction", "(1/(select/**/0))", "", "fallback"},
		{"hostile column with direction", "(1/(select/**/0))", "asc", "fallback"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := queryContext{OrderBy: c.orderBy, Order: c.order}
			assert.Equal(t, c.want, q.OrderClause("fallback", "created_at"))
		})
	}
}

// capturedSQL is a store whose queries are built but not run, and the SQL each
// one produced.
func capturedSQL(t *testing.T) (*store, *[]string) {
	t.Helper()
	db := newTestDB(t)
	var sqls []string
	record := func(tx *gorm.DB) {
		sqls = append(sqls, tx.Statement.SQL.String())
		// gorm skips this reset in DryRun, so a following query would reuse this SQL.
		tx.Statement.SQL.Reset()
		tx.Statement.Vars = nil
	}
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("capture", record))
	require.NoError(t, db.Callback().Row().After("gorm:row").Register("capture_row", record))
	return &store{db: db.Session(&gorm.Session{DryRun: true})}, &sqls
}

// The two queries that already validated their order keep doing so.
func TestUserCoursesAndCatalogIgnoreHostileOrder(t *testing.T) {
	const payload = "(1/(select/**/0))"
	t.Run("courses by order_by", func(t *testing.T) {
		s, sqls := capturedSQL(t)
		_, _ = s.getUserCourses(&queryContext{Ctx: context.Background(), PerPage: 10, Page: 1, UserID: 1, OrderBy: payload, Order: "asc"})
		assertOrderedBy(t, *sqls, "c.name asc")
	})
	t.Run("courses by order", func(t *testing.T) {
		s, sqls := capturedSQL(t)
		_, _ = s.getUserCourses(&queryContext{Ctx: context.Background(), PerPage: 10, Page: 1, UserID: 1, OrderBy: "course_name", Order: "desc," + payload})
		assertOrderedBy(t, *sqls, "c.name desc")
	})
	t.Run("catalog by order", func(t *testing.T) {
		s, sqls := capturedSQL(t)
		_, _ = s.getUserCatalog(1, nil, "", "desc,"+payload)
		assertOrderedBy(t, *sqls, "c.name desc")
	})
}

func assertOrderedBy(t *testing.T, sqls []string, want string) {
	t.Helper()
	require.NotEmpty(t, sqls, "query was not built")
	for _, q := range sqls {
		assert.NotContains(t, q, "select/**/0")
	}
	assert.True(t, slices.ContainsFunc(sqls, func(q string) bool { return strings.Contains(q, "ORDER BY "+want) }), "no query ordered by %q", want)
}

func TestOutcomesIgnoreHostileOrder(t *testing.T) {
	const payload = "(1/(select/**/0))"
	cases := []struct{ orderBy, order, want string }{
		{"type", "desc", "type desc"},
		{"created_at", "asc", "created_at asc"},
		{payload, "", "outcomes.created_at DESC"},
		{payload, "asc", "outcomes.created_at DESC"},
		{"type", "desc," + payload, "outcomes.created_at DESC"},
	}
	for _, c := range cases {
		t.Run(c.orderBy+" "+c.order, func(t *testing.T) {
			s, sqls := capturedSQL(t)
			_, _ = s.getOutcomesForUser(&queryContext{Ctx: context.Background(), PerPage: 10, Page: 1, UserID: 1, OrderBy: c.orderBy, Order: c.order}, "")
			assertOrderedBy(t, *sqls, c.want)
		})
	}
}
