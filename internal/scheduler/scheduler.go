// Package scheduler runs the module's cron jobs, as core's did before the move.
package scheduler

import (
	"time"

	"github.com/go-co-op/gocron/v2"
)

// startDelay holds back the at-start jobs: the workers start alongside the
// scheduler and subscribe only once they've checked migrations, and a job
// published before they subscribe is lost.
const startDelay = 60 * time.Second

// Scheduler is a module.Scheduler on gocron.
type Scheduler struct {
	gocron.Scheduler
}

// New starts a scheduler.
func New() (*Scheduler, error) {
	gs, err := gocron.NewScheduler()
	if err != nil {
		return nil, err
	}
	gs.Start()
	return &Scheduler{gs}, nil
}

// Cron runs task on a five-field cron schedule.
func (s *Scheduler) Cron(spec string, task func()) error {
	_, err := s.NewJob(gocron.CronJob(spec, false), gocron.NewTask(task))
	return err
}

// AtStart runs task once, startDelay after the scheduler starts.
func (s *Scheduler) AtStart(task func()) error {
	start := gocron.OneTimeJobStartDateTime(time.Now().Add(startDelay))
	_, err := s.NewJob(gocron.OneTimeJob(start), gocron.NewTask(task))
	return err
}

// Stop stops the jobs and shuts the scheduler down.
func (s *Scheduler) Stop() error {
	return s.Shutdown()
}
