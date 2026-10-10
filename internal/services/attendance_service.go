package services

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"dashboard-presensi/internal/config"
	"dashboard-presensi/internal/models"
)

// negativeCacheTTL: error BioTime ikut di-cache sebentar agar pengunjung berikutnya
// gagal cepat (fast-fail) alih-alih antri timeout penuh. Tidak melebihi cacheTTL.
const negativeCacheTTL = 15 * time.Second

var IndonesianShortMonths = [...]string{
	"Jan", "Feb", "Mar", "Apr", "Mei", "Jun",
	"Jul", "Ags", "Sep", "Okt", "Nov", "Des",
}

var IndonesianFullMonths = [...]string{
	"Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember",
}

func getShortMonth(m time.Month) string {
	if m >= 1 && m <= 12 {
		return IndonesianShortMonths[m-1]
	}
	return ""
}

func getFullMonth(m time.Month) string {
	if m >= 1 && m <= 12 {
		return IndonesianFullMonths[m-1]
	}
	return ""
}

// isSesiPagiSah / isSesiSoreSah menyamakan definisi "sesi sah" dengan EnrichSummaryWithAudit
// (scan sah + verifikasi + DL/Cuti/Sakit), agar bucket chart konsisten dengan kartu KPI.
func isSesiPagiSah(l models.DailyAttendanceLog) bool {
	return l.CheckInStatus == models.StatusTepatWaktu ||
		l.CheckInStatus == "Hadir Apel Pagi" ||
		models.IsExcusedLeaveStatus(l.CheckInStatus) ||
		l.IsApelPagiVerified
}

func isSesiSoreSah(l models.DailyAttendanceLog) bool {
	return l.CheckOutStatus == models.StatusSahApelSore ||
		l.CheckOutStatus == "Hadir Apel Sore" ||
		l.CheckOutStatus == "Sah Hadir Apel Sore" ||
		models.IsExcusedLeaveStatus(l.CheckOutStatus) ||
		l.IsApelSoreVerified
}

type AttendanceService struct {
	biotimeClient *BioTimeClient
	engine        *AttendanceEngine
	cache         *AttendanceCache
	cacheTTL      time.Duration
	ruleService   *RuleService
	auditService  *AuditService
	employeeDir   *EmployeeDirectory
	calendarSvc   *CalendarService
	snapshots     *SnapshotStore // nil bila SNAPSHOT_DIR tidak diset (fitur nonaktif)

	// flights coalesces concurrent cache-miss ke 1 fetch BioTime (thundering-herd guard).
	flightMu sync.Mutex
	flights  map[string]*flightCall
}

type flightCall struct {
	wg  sync.WaitGroup
	val *models.AttendanceSummary
}

func NewAttendanceService(client *BioTimeClient, engine *AttendanceEngine, cache *AttendanceCache) *AttendanceService {
	ttlSec, err := strconv.Atoi(config.GetEnv("CACHE_TTL_SECONDS", "60"))
	if err != nil || ttlSec <= 0 {
		ttlSec = 60
	}

	return &AttendanceService{
		biotimeClient: client,
		engine:        engine,
		cache:         cache,
		cacheTTL:      time.Duration(ttlSec) * time.Second,
		employeeDir:   NewEmployeeDirectory(10 * time.Minute),
		flights:       make(map[string]*flightCall),
		snapshots:     openSnapshotStore(),
	}
}

// openSnapshotStore mengaktifkan snapshot disk bila SNAPSHOT_DIR diset.
// Tanpa env tersebut fitur nonaktif (nil) agar test/unit tidak menulis ke disk.
func openSnapshotStore() *SnapshotStore {
	dir := strings.TrimSpace(config.GetEnv("SNAPSHOT_DIR", ""))
	if dir == "" {
		return nil
	}
	hours, err := strconv.Atoi(config.GetEnv("SNAPSHOT_MAX_AGE_HOURS", "24"))
	if err != nil || hours <= 0 {
		hours = 24
	}
	return NewSnapshotStore(dir, time.Duration(hours)*time.Hour)
}

// saveSnapshot menyimpan last-good ke disk (diabaikan bila fitur nonaktif).
func (s *AttendanceService) saveSnapshot(key string, summary *models.AttendanceSummary) {
	if s.snapshots != nil {
		s.snapshots.Save(key, summary)
	}
}

// loadSnapshot membaca last-good dari disk untuk cold-start / BioTime mati total.
func (s *AttendanceService) loadSnapshot(key string) (*models.AttendanceSummary, bool) {
	if s.snapshots == nil {
		return nil, false
	}
	return s.snapshots.Load(key)
}

// negativeTTL mengembalikan TTL untuk hasil error (dibatasi cacheTTL).
func (s *AttendanceService) negativeTTL() time.Duration {
	if s.cacheTTL < negativeCacheTTL {
		return s.cacheTTL
	}
	return negativeCacheTTL
}

// doSingleflight menjalankan fn sekali untuk banyak pemanggil concurrent per kunci.
// Aman terhadap panic fn: penunggu menerima ringkasan error, bukan nil.
func (s *AttendanceService) doSingleflight(key string, fn func() *models.AttendanceSummary) (out *models.AttendanceSummary) {
	s.flightMu.Lock()
	if c, ok := s.flights[key]; ok {
		s.flightMu.Unlock()
		c.wg.Wait()
		if c.val == nil {
			return &models.AttendanceSummary{Logs: []models.DailyAttendanceLog{}, ErrorMessage: "gagal memuat data presensi"}
		}
		return c.val
	}
	c := &flightCall{}
	c.wg.Add(1)
	s.flights[key] = c
	s.flightMu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("[AttendanceService] singleflight %s panic (diamankan): %v\n", key, r)
		}
		if out == nil {
			out = &models.AttendanceSummary{Logs: []models.DailyAttendanceLog{}, ErrorMessage: "gagal memuat data presensi"}
		}
		c.val = out
		c.wg.Done()
		s.flightMu.Lock()
		delete(s.flights, key)
		s.flightMu.Unlock()
	}()

	out = fn()
	return out
}

// InvalidateEmployeeCache menghapus mirror master (panggil setelah CRUD pegawai/mutasi).
func (s *AttendanceService) InvalidateEmployeeCache() {
	if s.employeeDir != nil {
		s.employeeDir.Invalidate()
	}
}

// InvalidateCache membuang seluruh ringkasan presensi ter-cache agar perubahan
// dispensasi/aturan langsung terlihat di refresh berikutnya.
func (s *AttendanceService) InvalidateCache() {
	if s.cache != nil {
		s.cache.Clear()
	}
}

// appendAlphaDaily menambahkan log sintetis Tidak Hadir untuk master yang nol tap hari itu.
func (s *AttendanceService) appendAlphaDaily(summary *models.AttendanceSummary, master []models.BioTimeEmployee, targetDate time.Time) {
	if len(master) == 0 {
		return
	}
	present := make(map[string]bool, len(summary.Logs))
	for _, l := range summary.Logs {
		present[l.EmpCode] = true
	}
	dateStr := targetDate.Format(time.DateOnly)
	dayName := IndonesianDayNames[targetDate.Weekday()]
	isFriday := targetDate.Weekday() == time.Friday
	for _, m := range master {
		if present[m.EmpCode] {
			continue
		}
		name := m.FullName
		if name == "" {
			name = m.FirstName
		}
		if name == "" {
			name = "Pegawai " + m.EmpCode
		}
		summary.Logs = append(summary.Logs, models.DailyAttendanceLog{
			EmpCode:         m.EmpCode,
			Name:            name,
			Department:      m.DepartmentName,
			Date:            dateStr,
			DayName:         dayName,
			IsFriday:        isFriday,
			CheckInDisplay:  "--:--:--",
			CheckInStatus:   models.StatusBelumScan,
			CheckOutDisplay: "--:--:--",
			CheckOutStatus:  models.StatusBelumScan,
			AuditStatus:     models.AuditStatusBelumDiverifikasi,
		})
	}
	summary.TotalPegawai = len(master)
}

// appendAlphaPeriod menambahkan log Alpha per hari kerja untuk master yang hilang di rentang.
func (s *AttendanceService) appendAlphaPeriod(summary *models.AttendanceSummary, master []models.BioTimeEmployee, startDate, endDate string) {
	if len(master) == 0 {
		return
	}
	start, err1 := time.ParseInLocation(time.DateOnly, startDate, time.Local)
	end, err2 := time.ParseInLocation(time.DateOnly, endDate, time.Local)
	if err1 != nil || err2 != nil || end.Before(start) {
		return
	}
	presentByDate := make(map[string]map[string]bool)
	for _, l := range summary.Logs {
		if _, ok := presentByDate[l.Date]; !ok {
			presentByDate[l.Date] = make(map[string]bool)
		}
		presentByDate[l.Date][l.EmpCode] = true
	}

	// Hari libur kedinasan dalam rentang tidak dihitung sebagai hari kerja
	// (tidak menambah Alpha massal, tidak masuk pembilang kepatuhan).
	var holidays map[string]models.CalendarEvent
	if s.calendarSvc != nil {
		holidays = s.calendarSvc.HolidayDatesInRange(start, end)
	}

	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		if _, isLibur := holidays[d.Format(time.DateOnly)]; isLibur {
			continue
		}
		dateStr := d.Format(time.DateOnly)
		dayName := IndonesianDayNames[d.Weekday()]
		isFriday := d.Weekday() == time.Friday
		present := presentByDate[dateStr]
		for _, m := range master {
			if present != nil && present[m.EmpCode] {
				continue
			}
			name := m.FullName
			if name == "" {
				name = m.FirstName
			}
			if name == "" {
				name = "Pegawai " + m.EmpCode
			}
			summary.Logs = append(summary.Logs, models.DailyAttendanceLog{
				EmpCode:         m.EmpCode,
				Name:            name,
				Department:      m.DepartmentName,
				Date:            dateStr,
				DayName:         dayName,
				IsFriday:        isFriday,
				CheckInDisplay:  "--:--:--",
				CheckInStatus:   models.StatusBelumScan,
				CheckOutDisplay: "--:--:--",
				CheckOutStatus:  models.StatusBelumScan,
				AuditStatus:     models.AuditStatusBelumDiverifikasi,
			})
		}
	}
	summary.TotalPegawai = len(master)
}

func (s *AttendanceService) SetAuditService(auditService *AuditService) {
	s.auditService = auditService
}

// EnrichSummaryWithAudit menyematkan status audit / dispensasi lapangan ke dalam summary dan menghitung ulang statistik
func (s *AttendanceService) EnrichSummaryWithAudit(summary *models.AttendanceSummary) {
	if s.auditService == nil {
		return
	}

	auditCache := make(map[string]map[string]*models.AuditRecord)
	var tepatWaktu, terlambat, apelSore, anomali int

	for i, item := range summary.Logs {
		cloned := item

		if _, ok := auditCache[cloned.Date]; !ok {
			auditCache[cloned.Date] = s.auditService.GetLatestAuditMap(cloned.Date)
		}
		auditMap := auditCache[cloned.Date]

		keyPagi := fmt.Sprintf("%s_Apel Pagi", cloned.EmpCode)
		keySore := fmt.Sprintf("%s_Apel Sore", cloned.EmpCode)

		if recPagi, ok := auditMap[keyPagi]; ok {
			cloned.AuditPagiStatus = recPagi.PhysicalStatus
			if recPagi.PhysicalStatus == models.AuditStatusHadirBarisan || recPagi.PhysicalStatus == models.StatusDisahkanHadir || recPagi.PhysicalStatus == "Disahkan Hadir" {
				cloned.IsApelPagiVerified = true
				cloned.CheckInStatus = models.StatusTepatWaktu // "Hadir Apel Pagi"
				cloned.IsAnomaly = false
				cloned.AnomalyReason = ""
			} else if recPagi.PhysicalStatus == models.AuditStatusAnomaliTapRun {
				cloned.IsAnomaly = true
				if recPagi.Notes != "" {
					cloned.AnomalyReason = recPagi.Notes
				}
			}
		}

		if recSore, ok := auditMap[keySore]; ok {
			cloned.AuditSoreStatus = recSore.PhysicalStatus
			if recSore.PhysicalStatus == models.AuditStatusHadirBarisan || recSore.PhysicalStatus == models.StatusDisahkanHadir || recSore.PhysicalStatus == "Disahkan Hadir" {
				cloned.IsApelSoreVerified = true
				cloned.CheckOutStatus = models.StatusSahApelSore // "Hadir Apel Sore" (gugurkan Lewat Batas Waktu)
				cloned.IsAnomaly = false
				cloned.AnomalyReason = ""
			} else if recSore.PhysicalStatus == models.AuditStatusAnomaliTapRun {
				cloned.IsAnomaly = true
				if recSore.Notes != "" {
					cloned.AnomalyReason = recSore.Notes
				}
			}
		}

		// Dispensasi cuti/dinas yang diakui sah (legacy + kode ekinerja).
		// Prioritas sesuai urutan ExcusedSessions agar catatan lama berperilaku sama.
		var excusedRec *models.AuditRecord
		for _, session := range models.ExcusedSessions() {
			if rec, ok := auditMap[fmt.Sprintf("%s_%s", cloned.EmpCode, session)]; ok {
				excusedRec = rec
				break
			}
		}

		if excusedRec != nil {
			display := excusedRec.PhysicalStatus
			if display == "" {
				display = excusedRec.Session
			}
			cloned.CheckInStatus = display
			cloned.CheckOutStatus = display
			cloned.IsAnomaly = false
			cloned.AnomalyReason = excusedRec.Notes
			cloned.AuditStatus = excusedRec.Session
		} else if cloned.IsApelPagiVerified || cloned.IsApelSoreVerified {
			cloned.AuditStatus = models.StatusDisahkanHadir
		} else {
			cloned.AuditStatus = models.AuditStatusBelumDiverifikasi
		}

		// Hitung ulang statistik berdasarkan status final
		isPagiSah := cloned.CheckInStatus == models.StatusTepatWaktu ||
			cloned.CheckInStatus == "Hadir Apel Pagi" ||
			models.IsExcusedLeaveStatus(cloned.CheckInStatus) ||
			cloned.IsApelPagiVerified

		if isPagiSah {
			tepatWaktu++
		} else if cloned.CheckInStatus == models.StatusTerlambat {
			terlambat++
		}

		isSoreSah := cloned.CheckOutStatus == models.StatusSahApelSore ||
			cloned.CheckOutStatus == "Hadir Apel Sore" ||
			models.IsExcusedLeaveStatus(cloned.CheckOutStatus) ||
			cloned.IsApelSoreVerified

		if isSoreSah {
			apelSore++
		}

		if cloned.IsAnomaly {
			anomali++
		}

		summary.Logs[i] = cloned
	}

	// Update summary counts
	summary.TepatWaktuCount = tepatWaktu
	summary.TerlambatCount = terlambat
	summary.ApelSoreCount = apelSore
	summary.AnomaliCount = anomali

	totalLogs := len(summary.Logs)
	if totalLogs > 0 {
		targetSesi := float64(totalLogs * 2)
		sesiTerpenuhi := float64(summary.TepatWaktuCount + summary.ApelSoreCount)
		summary.KepatuhanPercent = (sesiTerpenuhi / targetSesi) * 100
	}
}

// BioTimeClient mengembalikan client BioTime untuk operasi langsung
func (s *AttendanceService) BioTimeClient() *BioTimeClient {
	return s.biotimeClient
}

// SetRuleService menyinkronkan aturan jam kerja aktif ke engine
func (s *AttendanceService) SetRuleService(ruleSvc *RuleService) {
	s.ruleService = ruleSvc
	s.syncRule()
}

// SetCalendarService menyambungkan kalender kedinasan (libur/cuti/jam khusus) ke service + engine
func (s *AttendanceService) SetCalendarService(calSvc *CalendarService) {
	s.calendarSvc = calSvc
	s.engine.SetCalendar(calSvc)
}

// CalendarService mengembalikan kalender kedinasan aktif (bisa nil)
func (s *AttendanceService) CalendarService() *CalendarService {
	return s.calendarSvc
}

func (s *AttendanceService) syncRule() {
	if s.ruleService != nil {
		s.engine.SetRule(s.ruleService.GetActiveRule())
	}
}

// GetAttendanceByDate mengambil data presensi untuk tanggal tertentu dari BioTime (dengan cache per-tanggal)
func (s *AttendanceService) GetAttendanceByDate(ctx context.Context, targetDate time.Time) *models.AttendanceSummary {
	s.syncRule()
	dateStr := targetDate.Format(time.DateOnly)
	cacheKey := "date:" + dateStr
	if cached, ok := s.cache.Get(cacheKey); ok {
		return cached
	}

	// Hari libur kedinasan (kalender): bukan hari kerja — tidak fetch BioTime,
	// tidak menambah Alpha massal. Akhir pekan tetap lewat jalur lama.
	if targetDate.Weekday() != time.Saturday && targetDate.Weekday() != time.Sunday &&
		s.calendarSvc != nil && s.calendarSvc.IsHolidayDate(targetDate) {
		summary := &models.AttendanceSummary{
			Logs:        []models.DailyAttendanceLog{},
			HolidayName: s.holidayLabel(targetDate),
		}
		s.cache.Set(cacheKey, summary, s.cacheTTL)
		return summary
	}
	startTime := dateStr + " 00:00:00"
	endTime := dateStr + " 23:59:59"

	return s.doSingleflight(cacheKey, func() *models.AttendanceSummary {
		resp, err := s.biotimeClient.FetchAllTransactions(ctx, startTime, endTime, 1000, 2)
		if err != nil {
			log.Printf("[AttendanceService] Fetch error for date %s: %v\n", dateStr, err)
			if stale := s.cache.GetStale(cacheKey); stale != nil {
				return stale
			}
			if snap, ok := s.loadSnapshot(cacheKey); ok {
				log.Printf("[AttendanceService] Sajikan snapshot disk untuk %s\n", dateStr)
				s.cache.Set(cacheKey, snap, s.cacheTTL)
				return snap
			}
			errSummary := &models.AttendanceSummary{
				TotalPegawai: 0,
				Logs:         []models.DailyAttendanceLog{},
				ErrorMessage: err.Error(),
			}
			s.cache.Set(cacheKey, errSummary, s.negativeTTL())
			return errSummary
		}

		summary := s.engine.ProcessDailyAttendance(resp, targetDate)
		if s.employeeDir != nil {
			// Join master agar Alpha (nol tap) tercatat; gagal fetch master = fallback perilaku lama
			s.appendAlphaDaily(summary, s.employeeDir.GetActiveEmployees(ctx, s.biotimeClient), targetDate)
		}
		s.EnrichSummaryWithAudit(summary)
		// Total = master aktif bila tersedia; Kepatuhan = (Pagi+Sore)/(Total*2) sesuai ANALISA §4
		if s.employeeDir != nil && summary.TotalPegawai == 0 && len(summary.Logs) > 0 {
			summary.TotalPegawai = len(summary.Logs)
		}
		if summary.TotalPegawai > 0 {
			summary.KepatuhanPercent = float64(summary.TepatWaktuCount+summary.ApelSoreCount) / float64(summary.TotalPegawai*2) * 100
		}
		s.cache.Set(cacheKey, summary, s.cacheTTL)
		s.saveSnapshot(cacheKey, summary)
		return summary
	})
}

// GetAttendanceForPeriod mengambil data presensi untuk rentang periode (dengan cache per-rentang)
func (s *AttendanceService) GetAttendanceForPeriod(ctx context.Context, startDate, endDate string) *models.AttendanceSummary {
	s.syncRule()
	cacheKey := "period:" + startDate + ":" + endDate
	if cached, ok := s.cache.Get(cacheKey); ok {
		return cached
	}
	startTime := startDate + " 00:00:00"
	endTime := endDate + " 23:59:59"

	return s.doSingleflight(cacheKey, func() *models.AttendanceSummary {
		resp, err := s.biotimeClient.FetchAllTransactions(ctx, startTime, endTime, 1000, 30)
		if err != nil {
			log.Printf("[AttendanceService] Period fetch error (%s to %s): %v\n", startDate, endDate, err)
			if stale := s.cache.GetStale(cacheKey); stale != nil {
				return stale
			}
			if snap, ok := s.loadSnapshot(cacheKey); ok {
				log.Printf("[AttendanceService] Sajikan snapshot disk untuk %s s/d %s\n", startDate, endDate)
				s.cache.Set(cacheKey, snap, s.cacheTTL)
				return snap
			}
			errSummary := &models.AttendanceSummary{
				TotalPegawai: 0,
				Logs:         []models.DailyAttendanceLog{},
				ErrorMessage: err.Error(),
			}
			s.cache.Set(cacheKey, errSummary, s.negativeTTL())
			return errSummary
		}

		summary := s.engine.ProcessPeriodAttendance(resp)
		if s.employeeDir != nil {
			s.appendAlphaPeriod(summary, s.employeeDir.GetActiveEmployees(ctx, s.biotimeClient), startDate, endDate)
		}
		s.EnrichSummaryWithAudit(summary)
		// Untuk periode multi-hari: denominator = jumlah baris harian (master × hari kerja),
		// agar konsisten dengan rumus sesi ANALISA §4 (Enrich sudah hitung ini).
		if n := len(summary.Logs); n > 0 {
			summary.KepatuhanPercent = float64(summary.TepatWaktuCount+summary.ApelSoreCount) / float64(n*2) * 100
		}
		s.cache.Set(cacheKey, summary, s.cacheTTL)
		s.saveSnapshot(cacheKey, summary)
		return summary
	})
}

// holidayLabel menyusun label hari libur untuk banner ("Hari Libur Nasional: Nyepi").
func (s *AttendanceService) holidayLabel(date time.Time) string {
	if s.calendarSvc == nil {
		return ""
	}
	if ev := s.calendarSvc.HolidayOnDate(date); ev != nil {
		return models.CalendarEventLabel(ev.Type) + ": " + ev.Name
	}
	return ""
}

// GetTodayAttendance mengambil data presensi hari ini (dengan in-memory cache per-tanggal)
func (s *AttendanceService) GetTodayAttendance(ctx context.Context) *models.AttendanceSummary {
	today := time.Now()
	dateStr := today.Format(time.DateOnly)
	cacheKey := "date:" + dateStr
	if cached, ok := s.cache.Get(cacheKey); ok {
		return cached
	}

	summary := s.GetAttendanceByDate(ctx, today)

	// Jika tidak ada data sama sekali dari mesin untuk hari ini, fallback ke stale cache jika ada.
	// Hari libur kedinasan memang kosong by design — jangan sajikan data basi.
	if summary.TotalPegawai == 0 && summary.HolidayName == "" {
		if stale := s.cache.GetStale(cacheKey); stale != nil {
			return stale
		}
	}

	return summary
}

// GetAllTransactions mengambil seluruh riwayat transaksi dari BioTime
func (s *AttendanceService) GetAllTransactions(ctx context.Context, startTime, endTime string) ([]models.BioTimeTransaction, error) {
	if startTime == "" {
		startTime = fmt.Sprintf("%d-01-01 00:00:00", time.Now().Year())
	}
	if endTime == "" {
		endTime = time.Now().Format(time.DateOnly) + " 23:59:59"
	}

	resp, err := s.biotimeClient.FetchAllTransactions(ctx, startTime, endTime, 1000, 30)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// GetChartData mengembalikan data tren kehadiran berbasis DATA RIIL DARI BIOTIME
func (s *AttendanceService) GetChartData(ctx context.Context, period string) *models.ChartDataResponse {
	today := time.Now()
	period = strings.ToLower(strings.TrimSpace(period))

	switch period {
	case "daily":
		dayName := IndonesianDayNames[today.Weekday()]
		dateStr := fmt.Sprintf("%02d %s", today.Day(), getShortMonth(today.Month()))
		periodLabel := fmt.Sprintf("Periode: Harian - %s, %02d %s %d", dayName, today.Day(), getFullMonth(today.Month()), today.Year())

		categories := []string{
			fmt.Sprintf("06:00 (%s)", dateStr),
			fmt.Sprintf("07:00 (%s)", dateStr),
			fmt.Sprintf("08:00 (%s)", dateStr),
			fmt.Sprintf("12:00 (%s)", dateStr),
			fmt.Sprintf("13:00 (%s)", dateStr),
			fmt.Sprintf("15:00 (%s)", dateStr),
			fmt.Sprintf("16:00 (%s)", dateStr),
		}

		todaySummary := s.GetAttendanceByDate(ctx, today)
		hourCounts := map[int]int{}
		for _, log := range todaySummary.Logs {
			if log.CheckInTime != nil {
				h := log.CheckInTime.Hour()
				hourCounts[h]++
			}
			if log.CheckOutTime != nil {
				h := log.CheckOutTime.Hour()
				hourCounts[h]++
			}
		}

		tepatSeries := []int{hourCounts[6], hourCounts[7], hourCounts[8], hourCounts[12], hourCounts[13], hourCounts[15], hourCounts[16]}
		telatSeries := []int{0, 0, 0, 0, 0, 0, 0}

		distDaily := &models.ChartDistribution{
			TepatWaktu: todaySummary.TepatWaktuCount,
			Terlambat:  todaySummary.TerlambatCount,
			ApelSore:   todaySummary.ApelSoreCount,
			Anomali:    todaySummary.AnomaliCount,
			Series:     []int{todaySummary.TepatWaktuCount, todaySummary.TerlambatCount, todaySummary.ApelSoreCount, todaySummary.AnomaliCount},
			Labels:     []string{"Tepat Waktu (Pagi)", "Terlambat (Pagi)", "Hadir Apel Sore", "Temuan Ketidaksesuaian"},
		}

		return &models.ChartDataResponse{
			Period:      "daily",
			PeriodLabel: periodLabel,
			Categories:  categories,
			Series: []models.ChartSeries{
				{Name: "Hadir Sah (Apel Pagi / Sore)", Data: tepatSeries},
				{Name: "Terlambat / Temuan", Data: telatSeries},
			},
			Distribution: distDaily,
		}

	case "monthly":
		monthName := getFullMonth(today.Month())
		periodLabel := fmt.Sprintf("Periode: Bulanan - %s %d", monthName, today.Year())
		mShort := getShortMonth(today.Month())

		categories := []string{
			fmt.Sprintf("Mg 1 (01-07 %s)", mShort),
			fmt.Sprintf("Mg 2 (08-14 %s)", mShort),
			fmt.Sprintf("Mg 3 (15-21 %s)", mShort),
			fmt.Sprintf("Mg 4 (22-28 %s)", mShort),
		}

		// 1x fetch untuk sebulan penuh, lalu bucket per minggu dari memori (hemat 4x API call)
		yr, mo := today.Year(), today.Month()
		lastDay := time.Date(yr, mo+1, 0, 0, 0, 0, 0, time.Local).Day()
		mSummary := s.GetAttendanceForPeriod(ctx, fmt.Sprintf("%d-%02d-01", yr, mo), fmt.Sprintf("%d-%02d-%02d", yr, mo, lastDay))
		wHadir := []int{0, 0, 0, 0}
		wTelat := []int{0, 0, 0, 0}
		for _, lg := range mSummary.Logs {
			if len(lg.Date) < 10 {
				continue
			}
			day, _ := strconv.Atoi(lg.Date[8:10])
			wi := 3
			switch {
			case day <= 7:
				wi = 0
			case day <= 14:
				wi = 1
			case day <= 21:
				wi = 2
			}
			if isSesiPagiSah(lg) {
				wHadir[wi]++
			} else if lg.CheckInStatus == models.StatusTerlambat {
				wTelat[wi]++
			}
			if isSesiSoreSah(lg) {
				wHadir[wi]++
			}
			if lg.IsAnomaly {
				wTelat[wi]++
			}
		}
		distMonthly := &models.ChartDistribution{
			TepatWaktu: mSummary.TepatWaktuCount,
			Terlambat:  mSummary.TerlambatCount,
			ApelSore:   mSummary.ApelSoreCount,
			Anomali:    mSummary.AnomaliCount,
			Series:     []int{mSummary.TepatWaktuCount, mSummary.TerlambatCount, mSummary.ApelSoreCount, mSummary.AnomaliCount},
			Labels:     []string{"Tepat Waktu (Pagi)", "Terlambat (Pagi)", "Hadir Apel Sore", "Temuan Ketidaksesuaian"},
		}

		return &models.ChartDataResponse{
			Period:      "monthly",
			PeriodLabel: periodLabel,
			Categories:  categories,
			Series: []models.ChartSeries{
				{
					Name: "Hadir Sah (Apel Pagi / Sore)",
					Data: wHadir,
				},
				{
					Name: "Terlambat / Temuan",
					Data: wTelat,
				},
			},
			Distribution: distMonthly,
		}

	case "annual":
		periodLabel := fmt.Sprintf("Periode: Tahunan - Tahun %d", today.Year())
		yr := strconv.Itoa(today.Year())

		categories := []string{
			"Jan " + yr, "Feb " + yr, "Mar " + yr, "Apr " + yr,
			"Mei " + yr, "Jun " + yr, "Jul " + yr, "Ags " + yr,
			"Sep " + yr, "Okt " + yr, "Nov " + yr, "Des " + yr,
		}

		ySummary := s.GetAttendanceForPeriod(ctx, yr+"-01-01", yr+"-12-31")
		monthDataHadir := make([]int, 12)
		monthDataTemuan := make([]int, 12)
		for _, log := range ySummary.Logs {
			if len(log.Date) >= 7 {
				m, _ := strconv.Atoi(log.Date[5:7])
				if m >= 1 && m <= 12 {
					if isSesiPagiSah(log) || isSesiSoreSah(log) {
						// Hadir dihitung per sesi agar konsisten dengan Kepatuhan (2 sesi/hari)
						if isSesiPagiSah(log) {
							monthDataHadir[m-1]++
						}
						if isSesiSoreSah(log) {
							monthDataHadir[m-1]++
						}
					} else {
						monthDataTemuan[m-1]++
					}
					if log.IsAnomaly {
						monthDataTemuan[m-1]++
					}
				}
			}
		}

		distAnnual := &models.ChartDistribution{
			TepatWaktu: ySummary.TepatWaktuCount,
			Terlambat:  ySummary.TerlambatCount,
			ApelSore:   ySummary.ApelSoreCount,
			Anomali:    ySummary.AnomaliCount,
			Series:     []int{ySummary.TepatWaktuCount, ySummary.TerlambatCount, ySummary.ApelSoreCount, ySummary.AnomaliCount},
			Labels:     []string{"Tepat Waktu (Pagi)", "Terlambat (Pagi)", "Hadir Apel Sore", "Temuan Ketidaksesuaian"},
		}

		return &models.ChartDataResponse{
			Period:      "annual",
			PeriodLabel: periodLabel,
			Categories:  categories,
			Series: []models.ChartSeries{
				{
					Name: "Hadir Sah (Apel Pagi / Sore)",
					Data: monthDataHadir,
				},
				{
					Name: "Terlambat / Temuan",
					Data: monthDataTemuan,
				},
			},
			Distribution: distAnnual,
		}

	default: // weekly (Senin s/d Jumat)
		daysFromMonday := int(today.Weekday() - time.Monday)
		if daysFromMonday < 0 {
			daysFromMonday = 6
		}
		monday := today.AddDate(0, 0, -daysFromMonday)
		friday := monday.AddDate(0, 0, 4)

		periodLabel := fmt.Sprintf("Periode: Mingguan (%02d %s - %02d %s %d)",
			monday.Day(), getShortMonth(monday.Month()),
			friday.Day(), getShortMonth(friday.Month()),
			friday.Year(),
		)

		var weeklyCategories []string
		for i := 0; i < 5; i++ {
			d := monday.AddDate(0, 0, i)
			dName := IndonesianDayNames[d.Weekday()]
			weeklyCategories = append(weeklyCategories, fmt.Sprintf("%s (%02d %s)", dName, d.Day(), getShortMonth(d.Month())))
		}

		// 1x fetch Mon-Jumat, lalu bucket per hari dari memori (hemat 5x API call)
		wSummary := s.GetAttendanceForPeriod(ctx, monday.Format(time.DateOnly), friday.Format(time.DateOnly))
		byDate := make(map[string][]models.DailyAttendanceLog, 5)
		for _, lg := range wSummary.Logs {
			byDate[lg.Date] = append(byDate[lg.Date], lg)
		}
		hadirSeries := make([]int, 5)
		telatSeries := make([]int, 5)
		for i := 0; i < 5; i++ {
			d := monday.AddDate(0, 0, i)
			for _, lg := range byDate[d.Format(time.DateOnly)] {
				if isSesiPagiSah(lg) {
					hadirSeries[i]++
				} else if lg.CheckInStatus == models.StatusTerlambat {
					telatSeries[i]++
				}
				if isSesiSoreSah(lg) {
					hadirSeries[i]++
				}
				if lg.IsAnomaly {
					telatSeries[i]++
				}
			}
		}
		distWeekly := &models.ChartDistribution{
			TepatWaktu: wSummary.TepatWaktuCount,
			Terlambat:  wSummary.TerlambatCount,
			ApelSore:   wSummary.ApelSoreCount,
			Anomali:    wSummary.AnomaliCount,
			Series:     []int{wSummary.TepatWaktuCount, wSummary.TerlambatCount, wSummary.ApelSoreCount, wSummary.AnomaliCount},
			Labels:     []string{"Tepat Waktu (Pagi)", "Terlambat (Pagi)", "Hadir Apel Sore", "Temuan Ketidaksesuaian"},
		}

		return &models.ChartDataResponse{
			Period:      "weekly",
			PeriodLabel: periodLabel,
			Categories:  weeklyCategories,
			Series: []models.ChartSeries{
				{
					Name: "Hadir Sah (Apel Pagi / Sore)",
					Data: hadirSeries,
				},
				{
					Name: "Terlambat / Temuan",
					Data: telatSeries,
				},
			},
			Distribution: distWeekly,
		}
	}
}
