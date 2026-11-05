package providerplatforms

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// sampleCourses are the dev Canvas's courses, as core's seeder added them.
//
//go:embed seed_dev_courses.json
var sampleCourses []byte

// devCanvasURL is the local Canvas from core's `make canvas`, by its compose
// service name.
const devCanvasURL = "http://canvas"

// SeedDev fills a local database with sample Provider Platforms data for core's
// residents: the local Canvas from core's `make canvas`, its courses, and a
// year of activity, outcomes and milestones. It moved from core's dev seeder
// (`make seed`), which runs it after adding core's facilities and users. A
// failed run adds nothing, and a database that already has the local Canvas is
// left alone, so `make seed` can run again.
func SeedDev(ctx context.Context, db *gorm.DB) error {
	return seedDev(ctx, db, time.Now())
}

func seedDev(ctx context.Context, db *gorm.DB, now time.Time) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var residents []uint
		if err := tx.Table("users").Where("role = ? AND deleted_at IS NULL", "student").Pluck("id", &residents).Error; err != nil {
			return fmt.Errorf("find residents: %w", err)
		}
		if len(residents) == 0 {
			return errors.New("no residents: run core's seeder first")
		}
		var seeded int64
		if err := tx.Model(&ProviderPlatform{}).Where("base_url = ?", devCanvasURL).Count(&seeded).Error; err != nil {
			return fmt.Errorf("look for the local Canvas: %w", err)
		}
		if seeded > 0 {
			log.Info("The sample Provider Platforms data is already there")
			return nil
		}
		// base_url is the compose service name, not 127.0.0.1:3001, because the
		// calls come from inside the containers. The access key is minted per
		// machine, so it still has to be pasted in by hand.
		canvas := ProviderPlatform{
			Name:      "Canvas",
			BaseUrl:   devCanvasURL,
			AccountID: "1",
			Type:      CanvasOSS,
			State:     Enabled,
			AccessKey: "testing_key_replace_me",
		}
		if err := tx.Create(&canvas).Error; err != nil {
			return fmt.Errorf("create the Canvas provider: %w", err)
		}
		var courses []Course
		if err := json.Unmarshal(sampleCourses, &courses); err != nil {
			return fmt.Errorf("read the sample courses: %w", err)
		}
		for i := range courses {
			courses[i].ProviderPlatformID = canvas.ID
		}
		if err := tx.Create(&courses).Error; err != nil {
			return fmt.Errorf("create courses: %w", err)
		}
		for _, userID := range residents {
			for _, course := range courses {
				if err := seedResidentCourse(tx, userID, course, now); err != nil {
					return err
				}
			}
		}
		log.Info("Seeded the sample Provider Platforms data")
		return nil
	})
}

var (
	sampleOutcomes   = []OutcomeType{CollegeCredit, CourseCompletion, Certificate, PathwayCompletion}
	sampleMilestones = []MilestoneType{DiscussionPost, AssignmentSubmission, QuizSubmission, GradeReceived}
)

// seedResidentCourse enrolls a resident in course, with activity on four days
// in five over the last year, and either an outcome (40%) or an open
// milestone.
func seedResidentCourse(tx *gorm.DB, userID uint, course Course, now time.Time) error {
	externalID := strconv.Itoa(rand.Intn(1000))
	enrollment := UserEnrollment{CourseID: course.ID, UserID: userID, ExternalID: &externalID, CreatedAt: &course.CreatedAt}
	if err := tx.Create(&enrollment).Error; err != nil {
		return fmt.Errorf("enroll user %d in course %d: %w", userID, course.ID, err)
	}
	yearAgo := now.AddDate(-1, 0, 0)
	activities := make([]Activity, 0, 292)
	total := 0
	for day := range 365 {
		if day%5 == 0 {
			continue
		}
		delta := rand.Intn(1000)
		total += delta
		activities = append(activities, Activity{
			UserID:     userID,
			CourseID:   course.ID,
			Type:       CourseInteraction,
			TotalTime:  int64(total),
			TimeDelta:  int64(delta),
			ExternalID: strconv.Itoa(rand.Intn(1000)),
			CreatedAt:  yearAgo.AddDate(0, 0, day),
		})
	}
	if err := tx.Create(&activities).Error; err != nil {
		return fmt.Errorf("create activity for user %d: %w", userID, err)
	}
	if rand.Float32() < 0.4 {
		outcome := Outcome{UserID: userID, CourseID: course.ID, Type: sampleOutcomes[rand.Intn(len(sampleOutcomes))]}
		if err := tx.Create(&outcome).Error; err != nil {
			return fmt.Errorf("create an outcome for user %d: %w", userID, err)
		}
		return nil
	}
	milestone := Milestone{
		UserID:     userID,
		CourseID:   course.ID,
		Type:       sampleMilestones[rand.Intn(len(sampleMilestones))],
		ExternalID: fmt.Sprintf("seed-%d-%d", userID, course.ID),
	}
	if err := tx.Create(&milestone).Error; err != nil {
		return fmt.Errorf("create a milestone for user %d: %w", userID, err)
	}
	return nil
}
