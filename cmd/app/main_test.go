package main

import (
	"context"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		wantErr string
	}{
		{name: "no command", wantErr: "usage"},
		{name: "unknown command", args: []string{"nope"}, wantErr: "unknown command"},
		{
			name:    "serve needs the service token",
			args:    []string{"serve"},
			env:     map[string]string{"SERVICE_TOKEN": "", "CORE_URL": "http://core"},
			wantErr: "SERVICE_TOKEN",
		},
		{
			name:    "serve needs core's address",
			args:    []string{"serve"},
			env:     map[string]string{"SERVICE_TOKEN": "t", "CORE_URL": ""},
			wantErr: "CORE_URL",
		},
		{
			name:    "migrate connects to the database",
			args:    []string{"migrate"},
			env:     map[string]string{"APP_DSN": "host=127.0.0.1 port=1 user=x dbname=x sslmode=disable connect_timeout=1"},
			wantErr: "database",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			err := run(context.Background(), tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("run(%v) error = %v, want one containing %q", tt.args, err, tt.wantErr)
			}
		})
	}
}
