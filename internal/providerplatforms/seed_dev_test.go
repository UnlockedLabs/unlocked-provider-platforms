package providerplatforms

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The sample data is for core's residents only, all on the seeded Canvas, and a
// failed run adds nothing.
func TestSeedDev(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Exec(`DROP TABLE users`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE users (id integer PRIMARY KEY, username text, role text, deleted_at datetime)`).Error)
	ctx := t.Context()

	require.ErrorContains(t, seedDev(ctx, db, time.Now()), "run core's seeder first")
	var count int64
	require.NoError(t, db.Model(&ProviderPlatform{}).Count(&count).Error)
	require.Zero(t, count, "a failed run adds nothing")

	require.NoError(t, db.Exec(`INSERT INTO users (id, username, role) VALUES (1, 'admin', 'system_admin'), (2, 'jdoe', 'student'), (3, 'gone', 'student')`).Error)
	require.NoError(t, db.Exec(`UPDATE users SET deleted_at = CURRENT_TIMESTAMP WHERE id = 3`).Error)

	require.NoError(t, db.Migrator().RenameTable("outcomes", "outcomes_away"))
	require.NoError(t, db.Migrator().RenameTable("milestones", "milestones_away"))
	require.Error(t, seedDev(ctx, db, time.Now()))
	require.NoError(t, db.Model(&ProviderPlatform{}).Count(&count).Error)
	require.Zero(t, count, "a failed run adds nothing")
	require.NoError(t, db.Migrator().RenameTable("outcomes_away", "outcomes"))
	require.NoError(t, db.Migrator().RenameTable("milestones_away", "milestones"))

	require.NoError(t, seedDev(ctx, db, time.Now()))

	var providers []ProviderPlatform
	require.NoError(t, db.Find(&providers).Error)
	require.Len(t, providers, 1)
	assert.Equal(t, CanvasOSS, providers[0].Type)

	var courses []Course
	require.NoError(t, db.Find(&courses).Error)
	require.Len(t, courses, 10)
	for _, c := range courses {
		assert.Equal(t, providers[0].ID, c.ProviderPlatformID)
	}

	var enrolled []uint
	require.NoError(t, db.Model(&UserEnrollment{}).Distinct().Pluck("user_id", &enrolled).Error)
	assert.Equal(t, []uint{2}, enrolled, "only the resident who isn't deleted")

	var outcomes, milestones int64
	require.NoError(t, db.Model(&Outcome{}).Count(&outcomes).Error)
	require.NoError(t, db.Model(&Milestone{}).Count(&milestones).Error)
	assert.Equal(t, int64(10), outcomes+milestones, "one outcome or milestone per course")
	require.NoError(t, db.Model(&Activity{}).Count(&count).Error)
	assert.Equal(t, int64(10*292), count, "activity on four days in five for a year, per course")
}
