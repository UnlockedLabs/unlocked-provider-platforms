package main

import (
	"os"

	log "github.com/sirupsen/logrus"
)

// initLogging matches core's logging: JSON lines at LOG_LEVEL. A redo of
// logging for all services is tracked in EN-158.
func initLogging() {
	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(parseLogLevel())
}

func parseLogLevel() log.Level {
	env := os.Getenv("LOG_LEVEL")
	if env == "" {
		return log.InfoLevel
	}
	level, err := log.ParseLevel(env)
	if err != nil {
		log.Errorf("Error parsing log level: %v", err)
		return log.InfoLevel
	}
	return level
}
