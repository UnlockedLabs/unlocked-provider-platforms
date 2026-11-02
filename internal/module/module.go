// Package module holds the local equivalents of what the Provider Platforms
// code used from core's module package before it moved here (EN-206): the user
// types and clients, a route list the server registers, the job scheduler's
// shape, and the request log context.
package module

import (
	"context"
	"maps"
	"net/http"

	"github.com/UnlockedLabs/unlocked-contracts/coreapi"

	"github.com/nats-io/nats.go"
	"gorm.io/gorm"
)

// Core's answers, through the contracts' coreapi clients.
type (
	Users         = coreapi.Users
	LiveProviders = coreapi.LiveProviders
	User          = coreapi.User
	NewUser       = coreapi.NewUser
	ProviderUser  = coreapi.ProviderUser
)

// Route is one HTTP route, such as "GET /api/provider-platforms". Core checks
// who may call it before it proxies the request here, so the service doesn't.
type Route struct {
	Pattern string
	Handler http.Handler
}

// Router registers a module's routes.
type Router interface {
	Handle(Route)
}

// Scheduler runs a module's background jobs.
type Scheduler interface {
	// Cron runs task on a standard five-field cron schedule.
	Cron(spec string, task func()) error
	// AtStart runs task once, a minute after the scheduler starts, so the
	// workers that start alongside it are listening.
	AtStart(task func()) error
}

// Deps is what the service shares with the module.
type Deps struct {
	DB            *gorm.DB
	NATS          *nats.Conn
	Users         Users
	LiveProviders LiveProviders
}

type logKey struct{}

type requestLog struct {
	fields map[string]any
	add    func(key string, value any)
}

// WithLog returns a copy of ctx that carries the request's log: fields is what
// has been logged so far, and add adds a field to the request's log lines.
func WithLog(ctx context.Context, fields map[string]any, add func(key string, value any)) context.Context {
	return context.WithValue(ctx, logKey{}, requestLog{fields: fields, add: add})
}

// LogFields is a copy of the request's log fields, for a module's own log
// lines to start from. Outside a request it's empty.
func LogFields(ctx context.Context) map[string]any {
	rl, _ := ctx.Value(logKey{}).(requestLog)
	fields := make(map[string]any, len(rl.fields))
	maps.Copy(fields, rl.fields)
	return fields
}

// LogField adds a field to the request's log lines. Outside a request it does
// nothing.
func LogField(ctx context.Context, key string, value any) {
	if rl, ok := ctx.Value(logKey{}).(requestLog); ok {
		rl.add(key, value)
	}
}
