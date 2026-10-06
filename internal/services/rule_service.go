package services

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"dashboard-presensi/internal/config"
	"dashboard-presensi/internal/models"
)

type RuleService struct {
	mu       sync.RWMutex
	db       *sql.DB
	fallback *models.AttendanceRule
}

func NewRuleService(db *sql.DB) *RuleService {
	defaultRule := &models.AttendanceRule{
		ID:                   1,
		RuleName:             "Jam Kerja Normal ASN (SOP Resmi)",
		WorkStartTime:        config.GetEnv("WORK_START_TIME", "07:00:00"),
		CheckinToleranceTime: config.GetEnv("CHECKIN_TOLERANCE_TIME", "07:15:00"),
		CheckinWindowStart:   config.GetEnv("CHECKIN_WINDOW_START", "06:15:00"),
		CheckoutTimeMonThu:   config.GetEnv("CHECKOUT_TIME_MON_THU", "15:30:00"),
		CheckoutCutoffMonThu: config.GetEnv("CHECKOUT_CUTOFF_MON_THU", "15:45:00"),
		CheckoutTimeFri:      config.GetEnv("CHECKOUT_TIME_FRI", "16:00:00"),
		CheckoutCutoffFri:    config.GetEnv("CHECKOUT_CUTOFF_FRI", "16:15:00"),
		IsRamadhanMode:       false,
		IsActive:             true,
		UpdatedAt:            time.Now(),
	}

	return &RuleService{
		db:       db,
		fallback: defaultRule,
	}
}

// GetActiveRule mengambil aturan jam kerja aktif dari MariaDB
func (s *RuleService) GetActiveRule() *models.AttendanceRule {
	if s.db != nil {
		query := `SELECT id, rule_name, work_start_time, checkin_tolerance_time, checkin_window_start,
			checkout_time_mon_thu, checkout_cutoff_mon_thu, checkout_time_fri, checkout_cutoff_fri,
			is_ramadhan_mode, is_active, created_at, updated_at
			FROM attendance_rules
			WHERE is_active = 1
			ORDER BY id ASC LIMIT 1`
		row := s.db.QueryRow(query)
		var r models.AttendanceRule
		var ramadhanInt, activeInt int
		err := row.Scan(&r.ID, &r.RuleName, &r.WorkStartTime, &r.CheckinToleranceTime, &r.CheckinWindowStart,
			&r.CheckoutTimeMonThu, &r.CheckoutCutoffMonThu, &r.CheckoutTimeFri, &r.CheckoutCutoffFri,
			&ramadhanInt, &activeInt, &r.CreatedAt, &r.UpdatedAt)
		if err == nil {
			r.IsRamadhanMode = ramadhanInt == 1
			r.IsActive = activeInt == 1
			return &r
		}
		log.Printf("[RuleService] Query error, using fallback: %v\n", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fallback
}

// UpdateRule memperbarui aturan jam kerja
func (s *RuleService) UpdateRule(req models.UpdateAttendanceRuleRequest) (*models.AttendanceRule, error) {
	if req.WorkStartTime == "" || req.CheckoutTimeMonThu == "" || req.CheckoutTimeFri == "" {
		return nil, errors.New("jam masuk dan jam pulang wajib diisi")
	}

	ramadhanInt := 0
	if req.IsRamadhanMode {
		ramadhanInt = 1
	}

	if s.db != nil {
		query := `UPDATE attendance_rules SET
			rule_name = ?,
			work_start_time = ?,
			checkin_tolerance_time = ?,
			checkin_window_start = ?,
			checkout_time_mon_thu = ?,
			checkout_cutoff_mon_thu = ?,
			checkout_time_fri = ?,
			checkout_cutoff_fri = ?,
			is_ramadhan_mode = ?
			WHERE id = 1`
		_, err := s.db.Exec(query,
			req.RuleName,
			req.WorkStartTime,
			req.CheckinToleranceTime,
			req.CheckinWindowStart,
			req.CheckoutTimeMonThu,
			req.CheckoutCutoffMonThu,
			req.CheckoutTimeFri,
			req.CheckoutCutoffFri,
			ramadhanInt,
		)
		if err != nil {
			return nil, fmt.Errorf("gagal memperbarui aturan di MariaDB: %w", err)
		}
		return s.GetActiveRule(), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.fallback.RuleName = req.RuleName
	s.fallback.WorkStartTime = req.WorkStartTime
	s.fallback.CheckinToleranceTime = req.CheckinToleranceTime
	s.fallback.CheckinWindowStart = req.CheckinWindowStart
	s.fallback.CheckoutTimeMonThu = req.CheckoutTimeMonThu
	s.fallback.CheckoutCutoffMonThu = req.CheckoutCutoffMonThu
	s.fallback.CheckoutTimeFri = req.CheckoutTimeFri
	s.fallback.CheckoutCutoffFri = req.CheckoutCutoffFri
	s.fallback.IsRamadhanMode = req.IsRamadhanMode
	s.fallback.UpdatedAt = time.Now()

	return s.fallback, nil
}
