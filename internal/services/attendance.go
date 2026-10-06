package services

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"dashboard-presensi/internal/models"
)

var IndonesianDayNames = map[time.Weekday]string{
	time.Sunday:    "Minggu",
	time.Monday:    "Senin",
	time.Tuesday:   "Selasa",
	time.Wednesday: "Rabu",
	time.Thursday:  "Kamis",
	time.Friday:    "Jumat",
	time.Saturday:  "Sabtu",
}

// AttendanceEngine memproses raw punch log menjadi status presensi & apel resmi ASN
type AttendanceEngine struct {
	rule *models.AttendanceRule
}

func NewAttendanceEngine() *AttendanceEngine {
	return &AttendanceEngine{}
}

// SetRule menetapkan aturan jam kerja aktif
func (e *AttendanceEngine) SetRule(rule *models.AttendanceRule) {
	e.rule = rule
}

// ProcessDailyAttendance mengelompokkan transaksi per NIP/pegawai dan menerapkan aturan cutoff
func (e *AttendanceEngine) ProcessDailyAttendance(transactions []models.BioTimeTransaction, targetDate time.Time) *models.AttendanceSummary {
	dateStr := targetDate.Format(time.DateOnly)
	isFriday := targetDate.Weekday() == time.Friday
	dayName := IndonesianDayNames[targetDate.Weekday()]

	// Abaikan presensi di hari Sabtu & Minggu (Hari Libur Kedinasan ASN)
	if targetDate.Weekday() == time.Saturday || targetDate.Weekday() == time.Sunday {
		return &models.AttendanceSummary{
			Logs:         make([]models.DailyAttendanceLog, 0),
			TotalPegawai: 0,
		}
	}

	// 1. Kelompokkan transaksi per emp_code
	grouped := make(map[string][]models.BioTimeTransaction)
	empNames := make(map[string]string)
	empDepts := make(map[string]string)

	for _, tx := range transactions {
		// Filter transaksi hanya untuk target date jika ada
		if !strings.HasPrefix(tx.PunchTime, dateStr) && tx.PunchTime != "" {
			continue
		}

		grouped[tx.EmpCode] = append(grouped[tx.EmpCode], tx)

		name := strings.TrimSpace(tx.FirstName + " " + tx.LastName)
		if name == "" {
			name = "Pegawai " + tx.EmpCode
		}
		empNames[tx.EmpCode] = name
		empDepts[tx.EmpCode] = tx.Department
	}

	summary := &models.AttendanceSummary{
		Logs: make([]models.DailyAttendanceLog, 0),
	}

	// 2. Evaluasi aturan presensi per pegawai
	for empCode, txList := range grouped {
		// Urutkan transaksi berdasarkan jam punch_time secara kronologis
		sort.Slice(txList, func(i, j int) bool {
			return txList[i].PunchTime < txList[j].PunchTime
		})

		logEntry := e.evaluateEmployeePunches(empCode, empNames[empCode], empDepts[empCode], dateStr, dayName, isFriday, txList)
		summary.Logs = append(summary.Logs, logEntry)

		// Agregasi statistik
		if logEntry.CheckInStatus == models.StatusTepatWaktu {
			summary.TepatWaktuCount++
		} else if logEntry.CheckInStatus == models.StatusTerlambat {
			summary.TerlambatCount++
		}

		if logEntry.CheckOutStatus == models.StatusSahApelSore {
			summary.ApelSoreCount++
		}

		if logEntry.IsAnomaly {
			summary.AnomaliCount++
		}
	}

	summary.TotalPegawai = len(summary.Logs)
	if summary.TotalPegawai > 0 {
		targetSesi := float64(summary.TotalPegawai * 2)
		sesiTerpenuhi := float64(summary.TepatWaktuCount + summary.ApelSoreCount)
		summary.KepatuhanPercent = (sesiTerpenuhi / targetSesi) * 100
	}

	// Urutkan logs berdasarkan jam masuk (terbaru di atas)
	sort.Slice(summary.Logs, func(i, j int) bool {
		if summary.Logs[i].CheckInTime != nil && summary.Logs[j].CheckInTime != nil {
			return summary.Logs[i].CheckInTime.After(*summary.Logs[j].CheckInTime)
		}
		return summary.Logs[i].EmpCode < summary.Logs[j].EmpCode
	})

	return summary
}

// ProcessPeriodAttendance mengelompokkan transaksi per tanggal dan per pegawai untuk seluruh rentang periode
func (e *AttendanceEngine) ProcessPeriodAttendance(transactions []models.BioTimeTransaction) *models.AttendanceSummary {
	txByDate := make(map[string][]models.BioTimeTransaction)
	for _, tx := range transactions {
		if len(tx.PunchTime) >= 10 {
			dStr := tx.PunchTime[:10]
			txByDate[dStr] = append(txByDate[dStr], tx)
		}
	}

	var dates []string
	for d := range txByDate {
		dates = append(dates, d)
	}
	sort.Slice(dates, func(i, j int) bool {
		return dates[i] > dates[j] // terbaru di atas
	})

	combinedSummary := &models.AttendanceSummary{
		Logs: make([]models.DailyAttendanceLog, 0),
	}

	uniqueEmpCodes := make(map[string]bool)

	for _, dStr := range dates {
		tDate, err := time.Parse(time.DateOnly, dStr)
		if err != nil {
			continue
		}
		// Abaikan jika tanggal transaksi jatuh pada hari libur akhir pekan (Sabtu / Minggu)
		if tDate.Weekday() == time.Saturday || tDate.Weekday() == time.Sunday {
			continue
		}
		dailySummary := e.ProcessDailyAttendance(txByDate[dStr], tDate)
		combinedSummary.TepatWaktuCount += dailySummary.TepatWaktuCount
		combinedSummary.TerlambatCount += dailySummary.TerlambatCount
		combinedSummary.ApelSoreCount += dailySummary.ApelSoreCount
		combinedSummary.AnomaliCount += dailySummary.AnomaliCount
		combinedSummary.Logs = append(combinedSummary.Logs, dailySummary.Logs...)

		for _, log := range dailySummary.Logs {
			uniqueEmpCodes[log.EmpCode] = true
		}
	}

	combinedSummary.TotalPegawai = len(uniqueEmpCodes)

	totalLogs := len(combinedSummary.Logs)
	if totalLogs > 0 {
		targetSesi := float64(totalLogs * 2)
		sesiTerpenuhi := float64(combinedSummary.TepatWaktuCount + combinedSummary.ApelSoreCount)
		combinedSummary.KepatuhanPercent = (sesiTerpenuhi / targetSesi) * 100
	}

	return combinedSummary
}

// evaluateEmployeePunches mengevaluasi punch log untuk 1 pegawai
func (e *AttendanceEngine) evaluateEmployeePunches(
	empCode, name, dept, dateStr, dayName string,
	isFriday bool,
	txList []models.BioTimeTransaction,
) models.DailyAttendanceLog {
	logEntry := models.DailyAttendanceLog{
		EmpCode:         empCode,
		Name:            name,
		Department:      dept,
		Date:            dateStr,
		DayName:         dayName,
		IsFriday:        isFriday,
		AuditStatus:     models.AuditStatusBelumDiverifikasi,
		RawTransactions: txList,
	}

	// Default SOP jam kerja (Senin s/d Jumat)
	workStartStr := "07:00:00"
	toleranceEndStr := "07:15:00"
	checkoutOpenStr := "15:30:00"
	checkoutCutoffStr := "15:45:00"
	if isFriday {
		checkoutOpenStr = "16:00:00"
		checkoutCutoffStr = "16:15:00"
	}

	// Gunakan aturan dinamis dari database jika tersedia
	if e.rule != nil {
		if e.rule.WorkStartTime != "" {
			workStartStr = e.rule.WorkStartTime
		}
		if e.rule.CheckinToleranceTime != "" {
			toleranceEndStr = e.rule.CheckinToleranceTime
		}
		if isFriday {
			if e.rule.CheckoutTimeFri != "" {
				checkoutOpenStr = e.rule.CheckoutTimeFri
			}
			if e.rule.CheckoutCutoffFri != "" {
				checkoutCutoffStr = e.rule.CheckoutCutoffFri
			}
		} else {
			if e.rule.CheckoutTimeMonThu != "" {
				checkoutOpenStr = e.rule.CheckoutTimeMonThu
			}
			if e.rule.CheckoutCutoffMonThu != "" {
				checkoutCutoffStr = e.rule.CheckoutCutoffMonThu
			}
		}
	}

	workStartTime := e.parseTimeOnDate(dateStr, workStartStr)
	toleranceEndTime := e.parseTimeOnDate(dateStr, toleranceEndStr)
	checkoutOpenTime := e.parseTimeOnDate(dateStr, checkoutOpenStr)
	checkoutCutoffTime := e.parseTimeOnDate(dateStr, checkoutCutoffStr)

	midDay := e.parseTimeOnDate(dateStr, "12:00:00")

	var morningPunches []time.Time
	var afternoonPunches []time.Time

	for _, tx := range txList {
		t, err := time.ParseInLocation(time.DateTime, tx.PunchTime, time.Local)
		if err != nil {
			continue
		}
		if t.Before(midDay) {
			morningPunches = append(morningPunches, t)
		} else {
			afternoonPunches = append(afternoonPunches, t)
		}
	}

	// 1. Evaluasi Check-In (Pagi)
	if len(morningPunches) > 0 {
		firstPunch := morningPunches[0]
		logEntry.CheckInTime = &firstPunch
		logEntry.CheckInDisplay = firstPunch.Format("15:04:05")

		if firstPunch.After(toleranceEndTime) {
			logEntry.CheckInStatus = models.StatusTerlambat
			lateDuration := firstPunch.Sub(workStartTime)
			logEntry.LateMinutes = int(lateDuration.Minutes())
			if logEntry.LateMinutes < 1 {
				logEntry.LateMinutes = 1
			}
		} else {
			logEntry.CheckInStatus = models.StatusTepatWaktu
			logEntry.LateMinutes = 0
		}
	} else {
		logEntry.CheckInStatus = models.StatusBelumScan
		logEntry.CheckInDisplay = "--:--:--"
	}

	// 2. Evaluasi Check-Out / Apel Sore (pilih scan sah di jendela, bukan scan terakhir)
	if len(afternoonPunches) > 0 {
		var inWindow []time.Time
		var beforeOpen []time.Time
		var afterCutoff []time.Time
		for _, p := range afternoonPunches {
			if p.Before(checkoutOpenTime) {
				beforeOpen = append(beforeOpen, p)
			} else if !p.After(checkoutCutoffTime) {
				inWindow = append(inWindow, p)
			} else {
				afterCutoff = append(afterCutoff, p)
			}
		}

		switch {
		case len(inWindow) > 0:
			// Sah: gunakan scan paling awal di jendela apel sore
			firstValid := inWindow[0]
			logEntry.CheckOutTime = &firstValid
			logEntry.CheckOutDisplay = firstValid.Format("15:04:05")
			logEntry.CheckOutStatus = models.StatusSahApelSore
		case len(beforeOpen) > 0:
			// Tidak ada yang sah, tapi ada scan sebelum jam buka -> Pulang Cepat
			lastEarly := beforeOpen[len(beforeOpen)-1]
			logEntry.CheckOutTime = &lastEarly
			logEntry.CheckOutDisplay = lastEarly.Format("15:04:05")
			logEntry.CheckOutStatus = models.StatusPulangCepat
		default:
			// Hanya ada scan setelah cutoff -> Lewat Batas Mutlak
			firstLate := afterCutoff[0]
			logEntry.CheckOutTime = &firstLate
			logEntry.CheckOutDisplay = firstLate.Format("15:04:05")
			// Lewat batas mutlak (Hard Cutoff Exceeded)
			logEntry.CheckOutStatus = models.StatusLewatCutoff
			logEntry.IsAnomaly = true
			if isFriday {
				logEntry.AnomalyReason = fmt.Sprintf("Scan sore jam %s melebihi batas akhir presensi pulang hari Jumat (16:15:00)", logEntry.CheckOutDisplay)
			} else {
				logEntry.AnomalyReason = fmt.Sprintf("Scan sore jam %s melebihi batas akhir presensi pulang (15:45:00)", logEntry.CheckOutDisplay)
			}
		}
	} else {
		logEntry.CheckOutStatus = models.StatusBelumScan
		logEntry.CheckOutDisplay = "--:--:--"
	}

	return logEntry
}

func (e *AttendanceEngine) parseTimeOnDate(dateStr, timeStr string) time.Time {
	t, _ := time.ParseInLocation(time.DateTime, dateStr+" "+timeStr, time.Local)
	return t
}
