package scheduler

import "testing"

func TestCron(t *testing.T) {
	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Stop() })

	if err := s.Cron("0 1 * * 4", func() {}); err != nil {
		t.Fatalf("Cron(valid) error = %v", err)
	}
	if err := s.Cron("not a schedule", func() {}); err == nil {
		t.Fatal("Cron(invalid) error = nil, want one")
	}
	if err := s.AtStart(func() {}); err != nil {
		t.Fatalf("AtStart() error = %v", err)
	}
}
