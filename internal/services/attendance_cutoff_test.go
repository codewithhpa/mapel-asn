package services

import (
	"testing"
	"time"

	"dashboard-presensi/internal/models"
)

func mustParseDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		t.Fatalf("parse date %s: %v", s, err)
	}
	return d
}

func findLog(logs []models.DailyAttendanceLog, emp string) *models.DailyAttendanceLog {
	for i := range logs {
		if logs[i].EmpCode == emp {
			return &logs[i]
		}
	}
	return nil
}

// Verifikasi batas eksak Senin-Kamis: 15:30:00 sah, 15:45:00 sah, 15:45:01 ditolak.
func TestCutoff_MonThu_ExactBoundaries(t *testing.T) {
	e := NewAttendanceEngine()
	monday := mustParseDate(t, "2026-08-24") // Senin

	tx := []models.BioTimeTransaction{
		{EmpCode: "A", FirstName: "A", PunchTime: "2026-08-24 07:00:00"},
		{EmpCode: "A", FirstName: "A", PunchTime: "2026-08-24 15:30:00"},
		{EmpCode: "B", FirstName: "B", PunchTime: "2026-08-24 07:00:00"},
		{EmpCode: "B", FirstName: "B", PunchTime: "2026-08-24 15:45:00"},
		{EmpCode: "C", FirstName: "C", PunchTime: "2026-08-24 07:00:00"},
		{EmpCode: "C", FirstName: "C", PunchTime: "2026-08-24 15:45:01"},
		{EmpCode: "D", FirstName: "D", PunchTime: "2026-08-24 07:00:00"},
		{EmpCode: "D", FirstName: "D", PunchTime: "2026-08-24 15:29:59"},
	}
	s := e.ProcessDailyAttendance(tx, monday)
	for _, tc := range []struct {
		emp, want string
		anomaly   bool
	}{
		{"A", models.StatusSahApelSore, false},
		{"B", models.StatusSahApelSore, false},
		{"C", models.StatusLewatCutoff, true},
		{"D", models.StatusPulangCepat, false},
	} {
		l := findLog(s.Logs, tc.emp)
		if l == nil {
			t.Fatalf("log %s hilang", tc.emp)
		}
		if l.CheckOutStatus != tc.want {
			t.Errorf("emp %s: want %s got %s", tc.emp, tc.want, l.CheckOutStatus)
		}
		if l.IsAnomaly != tc.anomaly {
			t.Errorf("emp %s: anomaly want %v got %v", tc.emp, tc.anomaly, l.IsAnomaly)
		}
	}
}

// Verifikasi batas eksak Jumat: 16:00:00 sah, 16:15:00 sah, 16:15:01 ditolak.
func TestCutoff_Friday_ExactBoundaries(t *testing.T) {
	e := NewAttendanceEngine()
	friday := mustParseDate(t, "2026-08-28") // Jumat

	tx := []models.BioTimeTransaction{
		{EmpCode: "A", FirstName: "A", PunchTime: "2026-08-28 07:00:00"},
		{EmpCode: "A", FirstName: "A", PunchTime: "2026-08-28 16:00:00"},
		{EmpCode: "B", FirstName: "B", PunchTime: "2026-08-28 07:00:00"},
		{EmpCode: "B", FirstName: "B", PunchTime: "2026-08-28 16:15:00"},
		{EmpCode: "C", FirstName: "C", PunchTime: "2026-08-28 07:00:00"},
		{EmpCode: "C", FirstName: "C", PunchTime: "2026-08-28 16:15:01"},
		{EmpCode: "D", FirstName: "D", PunchTime: "2026-08-28 07:00:00"},
		{EmpCode: "D", FirstName: "D", PunchTime: "2026-08-28 15:59:59"},
	}
	s := e.ProcessDailyAttendance(tx, friday)
	for _, tc := range []struct {
		emp, want string
	}{
		{"A", models.StatusSahApelSore},
		{"B", models.StatusSahApelSore},
		{"C", models.StatusLewatCutoff},
		{"D", models.StatusPulangCepat},
	} {
		l := findLog(s.Logs, tc.emp)
		if l == nil {
			t.Fatalf("log %s hilang", tc.emp)
		}
		if l.CheckOutStatus != tc.want {
			t.Errorf("Jumat emp %s: want %s got %s", tc.emp, tc.want, l.CheckOutStatus)
		}
	}
}

// Regresi: 2x tap sore (sah + lewat) harus tetap Sah, bukan Lewat.
func TestCutoff_MultiTap_ValidThenLate_StaysValid(t *testing.T) {
	e := NewAttendanceEngine()
	monday := mustParseDate(t, "2026-08-24")
	tx := []models.BioTimeTransaction{
		{EmpCode: "X", FirstName: "X", PunchTime: "2026-08-24 07:05:00"},
		{EmpCode: "X", FirstName: "X", PunchTime: "2026-08-24 15:32:00"},
		{EmpCode: "X", FirstName: "X", PunchTime: "2026-08-24 16:30:00"},
	}
	s := e.ProcessDailyAttendance(tx, monday)
	l := findLog(s.Logs, "X")
	if l == nil {
		t.Fatal("log X hilang")
	}
	if l.CheckOutStatus != models.StatusSahApelSore {
		t.Errorf("multi-tap: want Sah, got %s (%s)", l.CheckOutStatus, l.CheckOutDisplay)
	}
	if l.IsAnomaly {
		t.Errorf("multi-tap sah tidak boleh anomali")
	}
	if l.CheckOutDisplay != "15:32:00" {
		t.Errorf("harus pakai scan sah pertama 15:32:00, got %s", l.CheckOutDisplay)
	}
}

// Pagi: 07:15:00 tepat waktu, 07:15:01 terlambat.
func TestCutoff_Morning_ToleranceBoundary(t *testing.T) {
	e := NewAttendanceEngine()
	monday := mustParseDate(t, "2026-08-24")
	tx := []models.BioTimeTransaction{
		{EmpCode: "P", FirstName: "P", PunchTime: "2026-08-24 07:15:00"},
		{EmpCode: "Q", FirstName: "Q", PunchTime: "2026-08-24 07:15:01"},
	}
	s := e.ProcessDailyAttendance(tx, monday)
	if l := findLog(s.Logs, "P"); l.CheckInStatus != models.StatusTepatWaktu {
		t.Errorf("07:15:00 harus Tepat Waktu, got %s", l.CheckInStatus)
	}
	if l := findLog(s.Logs, "Q"); l.CheckInStatus != models.StatusTerlambat {
		t.Errorf("07:15:01 harus Terlambat, got %s", l.CheckInStatus)
	}
}
