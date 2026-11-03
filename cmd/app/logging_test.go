package main

import (
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name, env string
		want      log.Level
	}{
		{"empty defaults to info", "", log.InfoLevel},
		{"valid", "debug", log.DebugLevel},
		{"invalid falls back to info", "loud", log.InfoLevel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", tt.env)
			if got := parseLogLevel(); got != tt.want {
				t.Errorf("parseLogLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInitLogging(t *testing.T) {
	std := log.StandardLogger()
	formatter, level := std.Formatter, std.GetLevel()
	t.Cleanup(func() { log.SetFormatter(formatter); log.SetLevel(level) })

	t.Setenv("LOG_LEVEL", "warn")
	initLogging()
	if _, ok := std.Formatter.(*log.JSONFormatter); !ok {
		t.Errorf("formatter = %T, want *logrus.JSONFormatter", std.Formatter)
	}
	if std.GetLevel() != log.WarnLevel {
		t.Errorf("level = %v, want warn", std.GetLevel())
	}
}
