// internal/models/log.go
package models

import (
	"time"

	"github.com/google/uuid"
)

type LogEntry struct {
	ID        int64          `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Level     string         `json:"level"`
	Service   string         `json:"service"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata"`
}

type AlertRule struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Pattern         string    `json:"pattern"`
	LevelFilter     *string   `json:"level_filter,omitempty"`
	ServiceFilter   *string   `json:"service_filter,omitempty"`
	CooldownMinutes int       `json:"cooldown_minutes"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
}

// HourCount represents log volume aggregated into 1-hour windows
type HourCount struct {
	Hour  time.Time `json:"hour"`
	Count int64     `json:"count"`
}

// StatsResult bundles distinct data sets for metric visualizations
type StatsResult struct {
	CountByLevel   map[string]int64 `json:"count_by_level"`
	CountByService map[string]int64 `json:"count_by_service"`
	LogsPerHour    []HourCount      `json:"logs_per_hour"`
}
