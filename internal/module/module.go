// Package module holds the local equivalents of what the Provider Platforms
// code used from core's module package before it moved here (EN-206): the user
// types and clients, a route list the server registers, the job scheduler's
// shape.
package module

import (
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
