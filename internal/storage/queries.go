// internal/storage/queries.go
package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/logstream/internal/models"
)

// Search performs dynamic full-text search and filtering across the logs table
func (s *Store) Search(ctx context.Context, q, service, level string, from, to time.Time, limit, offset int) ([]models.LogEntry, int64, error) {
	var whereClauses []string
	var args []any
	argPos := 1

	// Dynamically build the WHERE condition array and track positional arguments
	if q != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("search_vector @@ plainto_tsquery('english', $%d)", argPos))
		args = append(args, q)
		argPos++
	}
	if service != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("service = $%d", argPos))
		args = append(args, service)
		argPos++
	}
	if level != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("level = $%d", argPos))
		args = append(args, level)
		argPos++
	}
	if !from.IsZero() {
		whereClauses = append(whereClauses, fmt.Sprintf("timestamp >= $%d", argPos))
		args = append(args, from)
		argPos++
	}
	if !to.IsZero() {
		whereClauses = append(whereClauses, fmt.Sprintf("timestamp <= $%d", argPos))
		args = append(args, to)
		argPos++
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Query 1: Fetch total matching count for frontend pagination metrics
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM logs %s", whereSQL)
	var totalCount int64
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("failed running count query: %w", err)
	}

	// Query 2: Fetch paginated data subset using identical filters
	searchSQL := fmt.Sprintf(
		"SELECT id, timestamp, level, service, message, metadata FROM logs %s ORDER BY timestamp DESC LIMIT $%d OFFSET $%d",
		whereSQL, argPos, argPos+1,
	)
	searchArgs := append(args, limit, offset)

	rows, err := s.pool.Query(ctx, searchSQL, searchArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed executing search logs query: %w", err)
	}
	defer rows.Close()

	var logs []models.LogEntry
	for rows.Next() {
		var entry models.LogEntry
		err := rows.Scan(&entry.ID, &entry.Timestamp, &entry.Level, &entry.Service, &entry.Message, &entry.Metadata)
		if err != nil {
			return nil, 0, fmt.Errorf("failed scanning log row: %w", err)
		}
		// Ensure metadata map is initialized instead of returning null json arrays
		if entry.Metadata == nil {
			entry.Metadata = make(map[string]any)
		}
		logs = append(logs, entry)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating log rows: %w", err)
	}

	return logs, totalCount, nil
}

// GetLog returns a single target log record matching the explicit ID
func (s *Store) GetLog(ctx context.Context, id int64) (*models.LogEntry, error) {
	var entry models.LogEntry
	query := "SELECT id, timestamp, level, service, message, metadata FROM logs WHERE id = $1"

	err := s.pool.QueryRow(ctx, query, id).Scan(
		&entry.ID, &entry.Timestamp, &entry.Level, &entry.Service, &entry.Message, &entry.Metadata,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("log record not found with id %d: %w", id, err)
		}
		return nil, fmt.Errorf("failed fetching log profile: %w", err)
	}

	if entry.Metadata == nil {
		entry.Metadata = make(map[string]any)
	}

	return &entry, nil
}

// Stats runs multi-dimensional data aggregations for historical trends and structural overview charts
func (s *Store) Stats(ctx context.Context, from, to time.Time) (*models.StatsResult, error) {
	res := &models.StatsResult{
		CountByLevel:   make(map[string]int64),
		CountByService: make(map[string]int64),
		LogsPerHour:    []models.HourCount{},
	}

	// Aggregation A: Log entries parsed by severe validation type levels
	levelQuery := "SELECT level, COUNT(*) FROM logs WHERE timestamp BETWEEN $1 AND $2 GROUP BY level"
	lRows, err := s.pool.Query(ctx, levelQuery, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed compiling level metrics: %w", err)
	}
	defer lRows.Close()
	for lRows.Next() {
		var lvl string
		var count int64
		if err := lRows.Scan(&lvl, &count); err != nil {
			return nil, err
		}
		res.CountByLevel[lvl] = count
	}

	// Aggregation B: Top logs broken down across operational microservice tags
	serviceQuery := "SELECT service, COUNT(*) FROM logs WHERE timestamp BETWEEN $1 AND $2 GROUP BY service ORDER BY count DESC"
	sRows, err := s.pool.Query(ctx, serviceQuery, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed compiling service metrics: %w", err)
	}
	defer sRows.Close()
	for sRows.Next() {
		var svc string
		var count int64
		if err := sRows.Scan(&svc, &count); err != nil {
			return nil, err
		}
		res.CountByService[svc] = count
	}

	// Aggregation C: Time-series bucketing truncated down to 1-hour slots
	hourQuery := `
		SELECT date_trunc('hour', timestamp) AS hour, COUNT(*) 
		FROM logs 
		WHERE timestamp BETWEEN $1 AND $2 
		GROUP BY hour 
		ORDER BY hour`
	hRows, err := s.pool.Query(ctx, hourQuery, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed compiling time series hourly metrics: %w", err)
	}
	defer hRows.Close()
	for hRows.Next() {
		var hCount models.HourCount
		if err := hRows.Scan(&hCount.Hour, &hCount.Count); err != nil {
			return nil, err
		}
		res.LogsPerHour = append(res.LogsPerHour, hCount)
	}

	return res, nil
}

// ListServices collects a unique dictionary array of all actively registered system services
func (s *Store) ListServices(ctx context.Context) ([]string, error) {
	query := "SELECT DISTINCT service FROM logs ORDER BY service"
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed gathering unique service entries: %w", err)
	}
	defer rows.Close()

	var services []string
	for rows.Next() {
		var svc string
		if err := rows.Scan(&svc); err != nil {
			return nil, err
		}
		services = append(services, svc)
	}
	return services, nil
}

// --- Alert Rule CRUD ---

func (s *Store) ListRules(ctx context.Context) ([]models.AlertRule, error) {
	query := `SELECT id, name, pattern, level_filter, service_filter, cooldown_minutes, is_active, created_at 
			  FROM alert_rules ORDER BY created_at DESC`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list rules: %w", err)
	}
	defer rows.Close()

	var rules []models.AlertRule
	for rows.Next() {
		var r models.AlertRule
		if err := rows.Scan(&r.ID, &r.Name, &r.Pattern, &r.LevelFilter, &r.ServiceFilter, &r.CooldownMinutes, &r.IsActive, &r.CreatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (s *Store) GetRule(ctx context.Context, id uuid.UUID) (*models.AlertRule, error) {
	query := `SELECT id, name, pattern, level_filter, service_filter, cooldown_minutes, is_active, created_at 
			  FROM alert_rules WHERE id = $1`

	var r models.AlertRule
	err := s.pool.QueryRow(ctx, query, id).Scan(&r.ID, &r.Name, &r.Pattern, &r.LevelFilter, &r.ServiceFilter, &r.CooldownMinutes, &r.IsActive, &r.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("rule not found: %w", err)
		}
		return nil, err
	}
	return &r, nil
}

func (s *Store) CreateRule(ctx context.Context, rule models.AlertRule) (*models.AlertRule, error) {
	query := `INSERT INTO alert_rules (name, pattern, level_filter, service_filter, cooldown_minutes, is_active) 
			  VALUES ($1, $2, $3, $4, $5, $6) 
			  RETURNING id, created_at`

	err := s.pool.QueryRow(ctx, query, rule.Name, rule.Pattern, rule.LevelFilter, rule.ServiceFilter, rule.CooldownMinutes, rule.IsActive).Scan(&rule.ID, &rule.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create rule: %w", err)
	}
	return &rule, nil
}

func (s *Store) UpdateRule(ctx context.Context, id uuid.UUID, rule models.AlertRule) (*models.AlertRule, error) {
	query := `UPDATE alert_rules 
			  SET name=$1, pattern=$2, level_filter=$3, service_filter=$4, cooldown_minutes=$5, is_active=$6 
			  WHERE id=$7 
			  RETURNING id, name, pattern, level_filter, service_filter, cooldown_minutes, is_active, created_at`

	var r models.AlertRule
	err := s.pool.QueryRow(ctx, query, rule.Name, rule.Pattern, rule.LevelFilter, rule.ServiceFilter, rule.CooldownMinutes, rule.IsActive, id).Scan(
		&r.ID, &r.Name, &r.Pattern, &r.LevelFilter, &r.ServiceFilter, &r.CooldownMinutes, &r.IsActive, &r.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update rule: %w", err)
	}
	return &r, nil
}

func (s *Store) DeleteRule(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, "DELETE FROM alert_rules WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("rule not found")
	}
	return nil
}

// --- Alert History ---

func (s *Store) CreateAlert(ctx context.Context, ruleID uuid.UUID, logID int64, logTimestamp time.Time) error {
	query := `INSERT INTO alerts (rule_id, log_id, log_timestamp) VALUES ($1, $2, $3)`
	_, err := s.pool.Exec(ctx, query, ruleID, logID, logTimestamp)
	return err
}

func (s *Store) ListAlerts(ctx context.Context, ruleID *uuid.UUID, from, to time.Time, limit int) ([]models.AlertWithContext, error) {
	whereSQL := "WHERE a.fired_at BETWEEN $1 AND $2"
	args := []any{from, to}
	argPos := 3

	if ruleID != nil {
		whereSQL += fmt.Sprintf(" AND a.rule_id = $%d", argPos)
		args = append(args, *ruleID)
		argPos++
	}

	query := fmt.Sprintf(`
		SELECT a.id, a.rule_id, a.log_id, a.fired_at, 
		       r.name AS rule_name, r.pattern, 
		       l.level, l.service, l.message
		FROM alerts a
		JOIN alert_rules r ON a.rule_id = r.id
		JOIN logs l ON a.log_id = l.id AND a.log_timestamp = l.timestamp
		%s
		ORDER BY a.fired_at DESC
		LIMIT $%d`, whereSQL, argPos)

	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list alerts: %w", err)
	}
	defer rows.Close()

	var alerts []models.AlertWithContext
	for rows.Next() {
		var a models.AlertWithContext
		if err := rows.Scan(&a.ID, &a.RuleID, &a.LogID, &a.FiredAt, &a.RuleName, &a.Pattern, &a.Level, &a.Service, &a.Message); err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, nil
}
