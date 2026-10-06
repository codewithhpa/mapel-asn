package services

import (
	"testing"
	"time"

	"dashboard-presensi/internal/models"
)

func TestAttendanceEngine_MondayToThursdayRules(t *testing.T) {
	engine := NewAttendanceEngine()

	// 2026-08-24 is a Monday
	monday, err := time.ParseInLocation("2006-01-02", "2026-08-24", time.Local)
	if err != nil {
		t.Fatalf("Failed to parse date: %v", err)
	}

	transactions := []models.BioTimeTransaction{
		// Pegawai 1: Desy (Scan 07:10 WIB -> Tepat Waktu dengan toleransi, Scan Sore 15:35 -> Sah Apel Sore)
		{
			EmpCode:   "199304152018022001",
			FirstName: "Desy",
			PunchTime: "2026-08-24 07:10:14",
		},
		{
			EmpCode:   "199304152018022001",
			FirstName: "Desy",
			PunchTime: "2026-08-24 15:35:00",
		},
		// Pegawai 2: Ahmad (Scan 07:22 WIB -> Terlambat, Scan Sore 15:48 -> Lewat Cutoff 15:45)
		{
			EmpCode:   "198207092008011005",
			FirstName: "Ahmad",
			PunchTime: "2026-08-24 07:22:00",
		},
		{
			EmpCode:   "198207092008011005",
			FirstName: "Ahmad",
			PunchTime: "2026-08-24 15:48:30",
		},
	}

	summary := engine.ProcessDailyAttendance(transactions, monday)

	if summary.TotalPegawai != 2 {
		t.Errorf("Expected 2 employees, got %d", summary.TotalPegawai)
	}

	// Cari log masing-masing
	var desyLog, ahmadLog *models.DailyAttendanceLog
	for i := range summary.Logs {
		if summary.Logs[i].EmpCode == "199304152018022001" {
			desyLog = &summary.Logs[i]
		}
		if summary.Logs[i].EmpCode == "198207092008011005" {
			ahmadLog = &summary.Logs[i]
		}
	}

	// Cek Desy
	if desyLog == nil {
		t.Fatal("Desy log not found")
	}
	if desyLog.CheckInStatus != models.StatusTepatWaktu {
		t.Errorf("Expected Desy CheckInStatus '%s', got '%s'", models.StatusTepatWaktu, desyLog.CheckInStatus)
	}
	if desyLog.CheckOutStatus != models.StatusSahApelSore {
		t.Errorf("Expected Desy CheckOutStatus '%s', got '%s'", models.StatusSahApelSore, desyLog.CheckOutStatus)
	}
	if desyLog.IsAnomaly {
		t.Errorf("Expected Desy not anomaly")
	}

	// Cek Ahmad
	if ahmadLog == nil {
		t.Fatal("Ahmad log not found")
	}
	if ahmadLog.CheckInStatus != models.StatusTerlambat {
		t.Errorf("Expected Ahmad CheckInStatus '%s', got '%s'", models.StatusTerlambat, ahmadLog.CheckInStatus)
	}
	if ahmadLog.LateMinutes != 22 {
		t.Errorf("Expected Ahmad LateMinutes 22, got %d", ahmadLog.LateMinutes)
	}
	if ahmadLog.CheckOutStatus != models.StatusLewatCutoff {
		t.Errorf("Expected Ahmad CheckOutStatus '%s', got '%s'", models.StatusLewatCutoff, ahmadLog.CheckOutStatus)
	}
	if !ahmadLog.IsAnomaly {
		t.Errorf("Expected Ahmad to be flagged as anomaly (cutoff exceeded)")
	}

	if summary.TepatWaktuCount != 1 {
		t.Errorf("Expected 1 tepat waktu, got %d", summary.TepatWaktuCount)
	}
	if summary.TerlambatCount != 1 {
		t.Errorf("Expected 1 terlambat, got %d", summary.TerlambatCount)
	}
	if summary.AnomaliCount != 1 {
		t.Errorf("Expected 1 anomali, got %d", summary.AnomaliCount)
	}
}

func TestAttendanceEngine_FridayRules(t *testing.T) {
	engine := NewAttendanceEngine()

	// 2026-08-28 is a Friday
	friday, err := time.ParseInLocation("2006-01-02", "2026-08-28", time.Local)
	if err != nil {
		t.Fatalf("Failed to parse date: %v", err)
	}

	transactions := []models.BioTimeTransaction{
		// Pegawai 1: Budi (Jumat Senam: Scan 07:05 -> Tepat Waktu, Scan Sore 16:10 -> Sah Apel Sore batas 16:15)
		{
			EmpCode:   "1001",
			FirstName: "Budi",
			PunchTime: "2026-08-28 07:05:00",
		},
		{
			EmpCode:   "1001",
			FirstName: "Budi",
			PunchTime: "2026-08-28 16:10:00",
		},
		// Pegawai 2: Joko (Jumat: Scan 07:20 -> Telat 20 mnt dr 07:00, Scan Sore 16:20 -> Lewat Cutoff 16:15)
		{
			EmpCode:   "1002",
			FirstName: "Joko",
			PunchTime: "2026-08-28 07:20:00",
		},
		{
			EmpCode:   "1002",
			FirstName: "Joko",
			PunchTime: "2026-08-28 16:20:00",
		},
	}

	summary := engine.ProcessDailyAttendance(transactions, friday)

	var budiLog, jokoLog *models.DailyAttendanceLog
	for i := range summary.Logs {
		if summary.Logs[i].EmpCode == "1001" {
			budiLog = &summary.Logs[i]
		}
		if summary.Logs[i].EmpCode == "1002" {
			jokoLog = &summary.Logs[i]
		}
	}

	if budiLog.CheckInStatus != models.StatusTepatWaktu {
		t.Errorf("Expected Budi Tepat Waktu, got %s", budiLog.CheckInStatus)
	}
	if budiLog.CheckOutStatus != models.StatusSahApelSore {
		t.Errorf("Expected Budi Sah Apel Sore (16:10), got %s", budiLog.CheckOutStatus)
	}

	if jokoLog.CheckInStatus != models.StatusTerlambat {
		t.Errorf("Expected Joko Terlambat, got %s", jokoLog.CheckInStatus)
	}
	if jokoLog.LateMinutes != 20 {
		t.Errorf("Expected Joko LateMinutes 20 (from 07:00), got %d", jokoLog.LateMinutes)
	}
	if jokoLog.CheckOutStatus != models.StatusLewatCutoff {
		t.Errorf("Expected Joko Lewat Cutoff (16:20), got %s", jokoLog.CheckOutStatus)
	}
	if !jokoLog.IsAnomaly {
		t.Errorf("Expected Joko anomaly flag")
	}
}

func TestAttendanceEngine_WeekendRules(t *testing.T) {
	engine := NewAttendanceEngine()

	// 2026-08-29 is Saturday, 2026-08-30 is Sunday
	saturday, _ := time.ParseInLocation("2006-01-02", "2026-08-29", time.Local)
	sunday, _ := time.ParseInLocation("2006-01-02", "2026-08-30", time.Local)

	txSaturday := []models.BioTimeTransaction{
		{EmpCode: "1001", FirstName: "Budi", PunchTime: "2026-08-29 08:00:00"},
	}
	txSunday := []models.BioTimeTransaction{
		{EmpCode: "1002", FirstName: "Joko", PunchTime: "2026-08-30 09:30:00"},
	}

	summarySat := engine.ProcessDailyAttendance(txSaturday, saturday)
	if len(summarySat.Logs) != 0 || summarySat.TotalPegawai != 0 {
		t.Errorf("Expected Saturday attendance to be empty, got %d logs", len(summarySat.Logs))
	}

	summarySun := engine.ProcessDailyAttendance(txSunday, sunday)
	if len(summarySun.Logs) != 0 || summarySun.TotalPegawai != 0 {
		t.Errorf("Expected Sunday attendance to be empty, got %d logs", len(summarySun.Logs))
	}

	// Test Period Attendance includes weekday and weekend
	// 2026-08-28 (Fri) + 2026-08-29 (Sat)
	periodTx := []models.BioTimeTransaction{
		{EmpCode: "1001", FirstName: "Budi", PunchTime: "2026-08-28 07:05:00"},
		{EmpCode: "1001", FirstName: "Budi", PunchTime: "2026-08-29 08:00:00"},
	}
	periodSummary := engine.ProcessPeriodAttendance(periodTx)
	if len(periodSummary.Logs) != 1 {
		t.Errorf("Expected only Friday log in period attendance, got %d logs", len(periodSummary.Logs))
	}
	if periodSummary.Logs[0].Date != "2026-08-28" {
		t.Errorf("Expected log date 2026-08-28, got %s", periodSummary.Logs[0].Date)
	}
}
