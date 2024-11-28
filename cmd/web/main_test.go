package main

import (
	"pawprintpublic/internal/config"
	"testing"
)

func TestRun(t *testing.T) {
	var cfg config.Config
	_, err := run(cfg)
	if err == nil {
		t.Errorf("run() failed: %v", err)
	}

	cfg = config.Config{
		InProduction: false,
		UseCache:     false,
		// DBHost:       "localhost",
		// DBName:       "bookings",
		// DBUser:       "postgres",
		// DBPass:       "password",
		// DBPort:       "5432",
		// DBSSL:        "disable",
	}

	_, err = run(cfg)
	if err != nil {
		t.Errorf("run() failed: %v", err)
	}
}
