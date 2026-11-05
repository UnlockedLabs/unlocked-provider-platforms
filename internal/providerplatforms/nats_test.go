package providerplatforms

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/UnlockedLabs/unlocked-contracts/jobs"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// natsConn connects to the NATS server in NATS_TEST_URL, or skips the test.
// Run one with: docker run --rm -p 127.0.0.1:4222:4222 nats:2
func natsConn(t *testing.T) *nats.Conn {
	t.Helper()
	url := os.Getenv("NATS_TEST_URL")
	if url == "" {
		t.Skip("NATS_TEST_URL is not set")
	}
	nc, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	return nc
}

func taskFor(t *testing.T, w *worker, job jobs.Type, providerID uint) runnableTask {
	t.Helper()
	var task runnableTask
	require.NoError(t, w.store.db.
		Joins("JOIN cron_jobs ON cron_jobs.id = runnable_tasks.job_id").
		Where("cron_jobs.name = ? AND runnable_tasks.provider_platform_id = ?", string(job), providerID).
		First(&task).Error)
	return task
}

// The scheduler's publisher and the worker, over a real NATS server: a published
// job reaches its handler, the handler imports from the LMS, and the task goes
// back to pending with a new last_run.
func TestJobFlowOverNATS(t *testing.T) {
	nc := natsConn(t)
	t.Setenv("MIDDLEWARE_CRON_SCHEDULE", "0 22 * * *")
	db := newTestDB(t)
	fc := newFakeCanvas(t)
	fc.courses = []map[string]any{{"id": float64(101), "name": "Algebra", "course_code": "ALG-1"}}
	p := createCanvasProvider(t, db, fc)
	st := &store{db: db}
	w := &worker{store: st}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, w.subscribe(ctx, nc))
	jp := &jobPublisher{store: st, publish: nc.PublishMsg}

	jp.run(ctx, jobs.GetCourses, p.ID)

	require.Eventually(t, func() bool {
		return taskFor(t, w, jobs.GetCourses, p.ID).Status == taskPending
	}, 10*time.Second, 50*time.Millisecond, "the courses task never went back to pending")
	var courses []Course
	require.NoError(t, db.Find(&courses).Error)
	require.Len(t, courses, 1)
	assert.Equal(t, "Algebra", courses[0].Name)
	assert.False(t, taskFor(t, w, jobs.GetCourses, p.ID).LastRun.IsZero(), "a successful run moves last_run")

	for _, job := range []jobs.Type{jobs.GetMilestones, jobs.GetActivity} {
		other := createProvider(t, db, ProviderPlatform{Type: CanvasCloud, Name: string(job), AccountID: "1", AccessKey: "k", BaseUrl: fc.URL})
		jp.run(ctx, job, other.ID)
		require.Eventually(t, func() bool {
			return taskFor(t, w, job, other.ID).Status == taskPending
		}, 10*time.Second, 50*time.Millisecond, "the %s task never went back to pending", job)
	}
}

// Two workers share the queue group, so a job runs on one of them, not both.
func TestJobRunsOnOneWorker(t *testing.T) {
	nc := natsConn(t)
	t.Setenv("MIDDLEWARE_CRON_SCHEDULE", "0 22 * * *")
	db := newTestDB(t)
	fc := newFakeCanvas(t)
	fc.courses = []map[string]any{{"id": float64(101), "name": "Algebra", "course_code": "ALG-1"}}
	p := createCanvasProvider(t, db, fc)
	st := &store{db: db}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	for range 2 {
		require.NoError(t, (&worker{store: st}).subscribe(ctx, nc))
	}

	(&jobPublisher{store: st, publish: nc.PublishMsg}).run(ctx, jobs.GetCourses, p.ID)

	require.Eventually(t, func() bool {
		return taskFor(t, &worker{store: st}, jobs.GetCourses, p.ID).Status == taskPending
	}, 10*time.Second, 50*time.Millisecond)
	time.Sleep(500 * time.Millisecond)
	var listed int
	for _, r := range fc.Requests() {
		if strings.HasPrefix(r, "GET /api/v1/accounts/1/courses") {
			listed++
		}
	}
	assert.Equal(t, 1, listed, "the course list was fetched once, so one worker took the job")
}
