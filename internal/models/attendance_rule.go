package models

import "time"

// AttendanceRule merepresentasikan konfigurasi batas jam kerja dan toleransi presensi ASN
type AttendanceRule struct {
	ID                     int       `json:"id"`
	RuleName               string    `json:"rule_name"`
	WorkStartTime          string    `json:"work_start_time"`
	CheckinToleranceTime   string    `json:"checkin_tolerance_time"`
	CheckinWindowStart     string    `json:"checkin_window_start"`
	CheckoutTimeMonThu     string    `json:"checkout_time_mon_thu"`
	CheckoutCutoffMonThu   string    `json:"checkout_cutoff_mon_thu"`
	CheckoutTimeFri        string    `json:"checkout_time_fri"`
	CheckoutCutoffFri      string    `json:"checkout_cutoff_fri"`
	IsRamadhanMode         bool      `json:"is_ramadhan_mode"`
	IsActive               bool      `json:"is_active"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// UpdateAttendanceRuleRequest payload untuk memperbarui aturan jam kerja
type UpdateAttendanceRuleRequest struct {
	RuleName               string `json:"rule_name"`
	WorkStartTime          string `json:"work_start_time"`
	CheckinToleranceTime   string `json:"checkin_tolerance_time"`
	CheckinWindowStart     string `json:"checkin_window_start"`
	CheckoutTimeMonThu     string `json:"checkout_time_mon_thu"`
	CheckoutCutoffMonThu   string `json:"checkout_cutoff_mon_thu"`
	CheckoutTimeFri        string `json:"checkout_time_fri"`
	CheckoutCutoffFri      string `json:"checkout_cutoff_fri"`
	IsRamadhanMode         bool   `json:"is_ramadhan_mode"`
}
