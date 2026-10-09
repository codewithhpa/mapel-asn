package handlers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"dashboard-presensi/internal/config"
	"dashboard-presensi/internal/middleware"
	"dashboard-presensi/internal/models"
	"dashboard-presensi/internal/services"

	"github.com/gofiber/fiber/v2"
)

// loginLimiter membatasi brute-force: maks 10 percobaan / menit / IP (OWASP API Security).
var loginLimiter = struct {
	sync.Mutex
	attempts map[string][]time.Time
}{attempts: make(map[string][]time.Time)}

func loginAllowed(ip string) bool {
	const window = time.Minute
	const maxAttempts = 10
	now := time.Now()
	loginLimiter.Lock()
	defer loginLimiter.Unlock()
	kept := loginLimiter.attempts[ip][:0]
	for _, t := range loginLimiter.attempts[ip] {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= maxAttempts {
		loginLimiter.attempts[ip] = kept
		return false
	}
	loginLimiter.attempts[ip] = append(kept, now)
	return true
}

type DashboardHandler struct {
	attendanceService *services.AttendanceService
	auditService      *services.AuditService
	userService       *services.UserService
	ruleService       *services.RuleService
	biotimeClient     *services.BioTimeClient
}

func NewDashboardHandler(attendanceService *services.AttendanceService, auditService *services.AuditService, userService *services.UserService, ruleService *services.RuleService) *DashboardHandler {
	return &DashboardHandler{
		attendanceService: attendanceService,
		auditService:      auditService,
		userService:       userService,
		ruleService:       ruleService,
		biotimeClient:     attendanceService.BioTimeClient(),
	}
}

func SetupRoutes(app *fiber.App, svc *services.AttendanceService, auditSvc *services.AuditService, userSvc *services.UserService, ruleSvc *services.RuleService) {
	h := NewDashboardHandler(svc, auditSvc, userSvc, ruleSvc)

	// Rute Autentikasi Publik
	app.Get("/login", h.RenderLogin)
	app.Post("/login", h.PostLogin)
	app.Get("/logout", h.GetLogout)

	// Rute Publik (dengan OptionalAuth agar nama user muncul di topbar jika sudah login)
	app.Get("/", middleware.OptionalAuth(userSvc), h.RenderDashboard)
	app.Get("/presensi", middleware.OptionalAuth(userSvc), h.RenderPresensi)
	app.Get("/presensi/cetak", middleware.OptionalAuth(userSvc), h.RenderCetakPresensi)
	app.Get("/laporan/*", func(c *fiber.Ctx) error { return c.Redirect("/presensi") })
	app.Get("/laporan", func(c *fiber.Ctx) error { return c.Redirect("/presensi") })

	// Mode TV / Display Lobby (publik, selalu hari ini, tanpa sidebar & menu)
	app.Get("/tv", h.RenderTV)
	app.Get("/tv/presensi", h.RenderTVPresensi)

	// JSON APIs Publik
	app.Get("/api/presensi/summary", h.GetSummaryJSON)
	app.Get("/api/presensi/chart", h.GetChartDataJSON)
	app.Get("/api/presensi/distribution", h.GetDistributionJSON)
	app.Get("/api/biotime-status", h.GetBioTimeStatus)

	// Aturan Jam Kerja Presensi (Khusus Administrator - RoleID = 1)
	app.Get("/aturan-presensi", middleware.RequireAdmin(userSvc), h.RenderAturanPresensi)
	app.Get("/api/rules", h.GetAttendanceRuleJSON)
	app.Put("/api/rules", middleware.RequireAdmin(userSvc), h.PutAttendanceRule)
	app.Post("/api/rules", middleware.RequireAdmin(userSvc), h.PutAttendanceRule)

	// Rute Terproteksi Petugas (Login sebagai Petugas atau Admin)
	app.Get("/audit-barisan", func(c *fiber.Ctx) error { return c.Redirect("/dispensasi") })
	app.Get("/dispensasi", middleware.RequireAuth(userSvc), h.RenderDispensasi)
	app.Post("/api/dispensasi", middleware.RequireAuth(userSvc), h.PostSaveDispensasi)
	app.Post("/api/dispensasi/delete", middleware.RequireAuth(userSvc), h.PostDeleteDispensasi)
	app.Post("/api/audit/verify", middleware.RequireAuth(userSvc), h.PostSaveDispensasi) // backward compatibility
	app.Get("/api/audit/logs", middleware.RequireAuth(userSvc), h.GetAuditLogsJSON)

	// ==========================================
	// GRUP MASTER DATA (Khusus Administrator - RoleID = 1)
	// ==========================================
	// 1. Pegawai (ASN)
	app.Get("/pegawai", func(c *fiber.Ctx) error { return c.Redirect("/master/pegawai") })
	app.Get("/master/pegawai", middleware.RequireAdmin(userSvc), h.RenderMasterPegawai)
	app.Get("/api/pegawai", middleware.RequireAdmin(userSvc), h.GetEmployeesJSON)
	app.Post("/api/pegawai", middleware.RequireAdmin(userSvc), h.PostCreateEmployee)
	app.Put("/api/pegawai/:id", middleware.RequireAdmin(userSvc), h.PutUpdateEmployee)
	app.Delete("/api/pegawai/:id", middleware.RequireAdmin(userSvc), h.DeleteEmployee)
	app.Post("/api/pegawai/:id/delete", middleware.RequireAdmin(userSvc), h.DeleteEmployee)
	app.Get("/api/pegawai/template", middleware.RequireAdmin(userSvc), h.GetEmployeeTemplate)
	app.Post("/api/pegawai/import", middleware.RequireAdmin(userSvc), h.PostImportEmployees)

	// 2. Departemen (Bagian)
	app.Get("/master/departemen", middleware.RequireAdmin(userSvc), h.RenderMasterDepartemen)
	app.Get("/api/departemen", middleware.RequireAdmin(userSvc), h.GetDepartmentsJSON)
	app.Post("/api/departemen", middleware.RequireAdmin(userSvc), h.PostCreateDepartment)
	app.Put("/api/departemen/:id", middleware.RequireAdmin(userSvc), h.PutUpdateDepartment)
	app.Delete("/api/departemen/:id", middleware.RequireAdmin(userSvc), h.DeleteDepartment)
	app.Post("/api/departemen/:id/delete", middleware.RequireAdmin(userSvc), h.DeleteDepartment)

	// 3. Area (Lokasi Kerja)
	app.Get("/master/area", middleware.RequireAdmin(userSvc), h.RenderMasterArea)
	app.Get("/api/area", middleware.RequireAdmin(userSvc), h.GetAreasJSON)
	app.Post("/api/area", middleware.RequireAdmin(userSvc), h.PostCreateArea)
	app.Put("/api/area/:id", middleware.RequireAdmin(userSvc), h.PutUpdateArea)
	app.Delete("/api/area/:id", middleware.RequireAdmin(userSvc), h.DeleteArea)
	app.Post("/api/area/:id/delete", middleware.RequireAdmin(userSvc), h.DeleteArea)

	// 4. Jabatan (Position)
	app.Get("/master/jabatan", middleware.RequireAdmin(userSvc), h.RenderMasterJabatan)
	app.Get("/api/jabatan", middleware.RequireAdmin(userSvc), h.GetPositionsJSON)
	app.Post("/api/jabatan", middleware.RequireAdmin(userSvc), h.PostCreatePosition)
	app.Put("/api/jabatan/:id", middleware.RequireAdmin(userSvc), h.PutUpdatePosition)
	app.Delete("/api/jabatan/:id", middleware.RequireAdmin(userSvc), h.DeletePosition)
	app.Post("/api/jabatan/:id/delete", middleware.RequireAdmin(userSvc), h.DeletePosition)

	// 5. Mutasi & Resign ASN
	app.Get("/master/mutasi", middleware.RequireAdmin(userSvc), h.RenderMasterMutasi)
	app.Get("/api/mutasi", middleware.RequireAdmin(userSvc), h.GetResignsJSON)
	app.Post("/api/mutasi", middleware.RequireAdmin(userSvc), h.PostCreateResign)
	app.Delete("/api/mutasi/:id", middleware.RequireAdmin(userSvc), h.DeleteResign)
	app.Post("/api/mutasi/:id/delete", middleware.RequireAdmin(userSvc), h.DeleteResign)
	app.Post("/api/mutasi/reinstatement", middleware.RequireAdmin(userSvc), h.PostReinstatementResign)

	// 6. Kelola Device Face Recognition (perintah transfer BioTime)
	app.Get("/device", middleware.RequireAdmin(userSvc), h.RenderDevice)
	app.Get("/api/device", middleware.RequireAdmin(userSvc), h.GetTerminalsJSON)
	app.Post("/api/device/upload", middleware.RequireAdmin(userSvc), h.PostUploadTransaction)
	app.Post("/api/device/:id/upload", middleware.RequireAdmin(userSvc), h.PostUploadTransaction)
	app.Post("/api/device/resync", middleware.RequireAdmin(userSvc), h.PostResyncEmployees)

	// CRUD Petugas (Khusus Administrator - RoleID = 1)
	app.Get("/petugas", middleware.RequireAdmin(userSvc), h.RenderPetugas)
	app.Post("/api/petugas", middleware.RequireAdmin(userSvc), h.PostCreateUser)
	app.Put("/api/petugas/:id", middleware.RequireAdmin(userSvc), h.PutUpdateUser)
	app.Delete("/api/petugas/:id", middleware.RequireAdmin(userSvc), h.DeleteUser)
	app.Post("/api/petugas/:id/delete", middleware.RequireAdmin(userSvc), h.DeleteUser)

	// CRUD Roles / Hak Akses (Khusus Administrator - RoleID = 1)
	app.Get("/roles", middleware.RequireAdmin(userSvc), h.RenderRoles)
	app.Get("/api/roles", h.GetRolesJSON)
	app.Get("/api/roles/:id", middleware.RequireAdmin(userSvc), h.GetRoleDetailJSON)
	app.Post("/api/roles", middleware.RequireAdmin(userSvc), h.PostCreateRole)
	app.Put("/api/roles/:id", middleware.RequireAdmin(userSvc), h.PutUpdateRole)
	app.Delete("/api/roles/:id", middleware.RequireAdmin(userSvc), h.DeleteRole)
	app.Post("/api/roles/:id/delete", middleware.RequireAdmin(userSvc), h.DeleteRole)
}

// 1. Auth Handlers
func (h *DashboardHandler) RenderLogin(c *fiber.Ctx) error {
	if token := c.Cookies("auth_session"); token != "" {
		if _, ok := h.userService.GetSessionUser(token); ok {
			return c.Redirect("/dispensasi")
		}
	}
	return c.Render("login", fiber.Map{
		"BioTimeVersion": config.GetEnv("BIOTIME_VERSION", "8.5"),
	})
}

func (h *DashboardHandler) PostLogin(c *fiber.Ctx) error {
	if !loginAllowed(c.IP()) {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"status":  "error",
			"message": "Terlalu banyak percobaan login. Coba lagi dalam 1 menit.",
		})
	}
	var req models.UserLoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "Format data login tidak valid",
		})
	}

	user, err := h.userService.Authenticate(req.Username, req.Password)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}

	token := h.userService.CreateSession(user.ID)
	c.Cookie(&fiber.Cookie{
		Name:     "auth_session",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HTTPOnly: true,
		SameSite: "Lax",
	})

	return c.JSON(fiber.Map{
		"status":   "success",
		"message":  "Login berhasil",
		"user":     user.ToDTO(),
		"redirect": "/dispensasi",
	})
}

func (h *DashboardHandler) GetLogout(c *fiber.Ctx) error {
	h.userService.DeleteSession(c.Cookies("auth_session"))
	c.ClearCookie("auth_session")
	return c.Redirect("/login")
}

type MonthOption struct {
	Value    string
	Label    string
	IsActive bool
}

func (h *DashboardHandler) getAvailableMonths(selectedMonth string) []MonthOption {
	now := time.Now()
	var options []MonthOption

	// Secara dinamis menghitung 12 bulan ke belakang dari waktu saat ini (Bulan Berjalan di posisi teratas)
	for i := 0; i < 12; i++ {
		t := now.AddDate(0, -i, 0)
		val := fmt.Sprintf("%d-%02d", t.Year(), t.Month())
		label := fmt.Sprintf("%s %d", getIndonesianMonth(t.Month()), t.Year())
		if i == 0 {
			label += " (Bulan Berjalan)"
		}
		options = append(options, MonthOption{
			Value:    val,
			Label:    label,
			IsActive: val == selectedMonth,
		})
	}
	return options
}

// Helper untuk mem-parsing parameter filter periode, tanggal, dan bulan
func (h *DashboardHandler) parseFilterParams(c *fiber.Ctx) (startDate, endDate, periodLabel, selectedMonth, periodType string) {
	now := time.Now()
	monthParam := c.Query("month")
	startParam := c.Query("start_date")
	endParam := c.Query("end_date")
	periodParam := c.Query("period")
	dateParam := c.Query("date")

	// 1. Filter per Bulan (misal: 2026-08 atau 2026-06)
	if monthParam != "" {
		if t, err := time.Parse("2006-01", monthParam); err == nil {
			startDate = fmt.Sprintf("%d-%02d-01", t.Year(), t.Month())
			lastDay := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.Local).Day()
			endDate = fmt.Sprintf("%d-%02d-%02d", t.Year(), t.Month(), lastDay)
			periodLabel = fmt.Sprintf("Bulan %s %d", getIndonesianMonth(t.Month()), t.Year())
			selectedMonth = monthParam
			periodType = "month"
			return
		}
	}

	// 2. Filter Rentang Tanggal Kustom (Start Date s/d End Date)
	if startParam != "" && endParam != "" {
		startDate = startParam
		endDate = endParam
		periodLabel = fmt.Sprintf("Periode %s s/d %s", startDate, endDate)
		periodType = "custom"
		return
	}

	// 3. Pilihan Cepat: Semua Data (Tahun Berjalan Dinamis)
	if periodParam == "all" || dateParam == "all" {
		startDate = fmt.Sprintf("%d-01-01", now.Year())
		endDate = now.Format(time.DateOnly)
		periodLabel = fmt.Sprintf("Semua Riwayat Log Presensi (Tahun %d)", now.Year())
		periodType = "all"
		return
	}

	// 4. Pilihan Cepat: Bulan Ini
	if periodParam == "this_month" {
		startDate = fmt.Sprintf("%d-%02d-01", now.Year(), now.Month())
		endDate = now.Format(time.DateOnly)
		periodLabel = fmt.Sprintf("Bulan Ini (%s %d)", getIndonesianMonth(now.Month()), now.Year())
		selectedMonth = fmt.Sprintf("%d-%02d", now.Year(), now.Month())
		periodType = "this_month"
		return
	}

	// 5. Pilihan Cepat: Minggu Ini
	if periodParam == "this_week" {
		daysFromMonday := int(now.Weekday() - time.Monday)
		if daysFromMonday < 0 {
			daysFromMonday = 6
		}
		monday := now.AddDate(0, 0, -daysFromMonday)
		startDate = monday.Format(time.DateOnly)
		endDate = now.Format(time.DateOnly)
		periodLabel = fmt.Sprintf("Minggu Ini (%s s/d %s)", startDate, endDate)
		periodType = "this_week"
		return
	}

	// 6. Tanggal Tunggal Spesifik (misal: 2026-08-21)
	if dateParam != "" {
		startDate = dateParam
		endDate = dateParam
		if t, err := time.Parse(time.DateOnly, dateParam); err == nil {
			periodLabel = fmt.Sprintf("%s, %02d %s %d", services.IndonesianDayNames[t.Weekday()], t.Day(), getIndonesianMonth(t.Month()), t.Year())
		} else {
			periodLabel = dateParam
		}
		periodType = "date"
		return
	}

	// 7. Default: Hari Ini
	todayStr := now.Format(time.DateOnly)
	startDate = todayStr
	endDate = todayStr
	periodLabel = fmt.Sprintf("%s, %02d %s %d", services.IndonesianDayNames[now.Weekday()], now.Day(), getIndonesianMonth(now.Month()), now.Year())
	periodType = "today"
	return
}

// 2. Dashboard & Presensi Handlers dengan Filter Periode & Bulan
func (h *DashboardHandler) RenderDashboard(c *fiber.Ctx) error {
	ctx := context.Background()
	startDate, endDate, periodLabel, selectedMonth, periodType := h.parseFilterParams(c)

	var summary *models.AttendanceSummary
	if startDate == endDate {
		t, _ := time.Parse(time.DateOnly, startDate)
		summary = h.attendanceService.GetAttendanceByDate(ctx, t)
	} else {
		summary = h.attendanceService.GetAttendanceForPeriod(ctx, startDate, endDate)
	}

	// Pola TV: BioTime mati bukan error page — sajikan snapshot/stale terakhir
	// (atau empty state) dengan banner offline. ErrorMessage mentah tidak pernah
	// dirender ke operator; banner generik yang tampil via IsOffline.
	var activeRule *models.AttendanceRule
	if h.ruleService != nil {
		activeRule = h.ruleService.GetActiveRule()
	}

	statusFilter := normalizeStatusFilter(c.Query("status"))

	return c.Render("index", fiber.Map{
		"BioTimeVersion":  config.GetEnv("BIOTIME_VERSION", "9.5"),
		"Title":           "Monitoring Apel",
		"Summary":         summary,
		"FilteredLogs":    filterLogsByStatus(summary.Logs, statusFilter),
		"StatusTabs":      buildStatusTabs(c, summary.Logs, statusFilter),
		"ActiveStatus":    statusFilter,
		"PeriodTabs":      buildPeriodTabs(c, periodType),
		"PrintURL":        buildQueryURL(c, "/presensi/cetak", nil),
		"Rule":            activeRule,
		"DateDisplay":     periodLabel,
		"ActiveMenu":      "dashboard",
		"CurrentUser":     c.Locals("currentUser"),
		"StartDate":       startDate,
		"EndDate":         endDate,
		"SelectedMonth":   selectedMonth,
		"PeriodType":      periodType,
		"IsMultiDay":      startDate != endDate,
		"IsOffline":       summary.ErrorMessage != "",
		"HasData":         len(summary.Logs) > 0,
		"AvailableMonths": h.getAvailableMonths(selectedMonth),
		"CurrentYear":     time.Now().Year(),
	}, "layouts/main")
}

func (h *DashboardHandler) RenderPresensi(c *fiber.Ctx) error {
	ctx := context.Background()
	startDate, endDate, periodLabel, selectedMonth, periodType := h.parseFilterParams(c)

	var summary *models.AttendanceSummary
	if startDate == endDate {
		t, _ := time.Parse(time.DateOnly, startDate)
		summary = h.attendanceService.GetAttendanceByDate(ctx, t)
	} else {
		summary = h.attendanceService.GetAttendanceForPeriod(ctx, startDate, endDate)
	}

	// Pola TV: BioTime mati bukan error page — sajikan snapshot/stale terakhir
	// (atau empty state) dengan banner offline.
	return c.Render("presensi", fiber.Map{
		"BioTimeVersion":  config.GetEnv("BIOTIME_VERSION", "9.5"),
		"Title":           "Presensi ASN",
		"Summary":         summary,
		"DateDisplay":     periodLabel,
		"ActiveMenu":      "presensi",
		"CurrentUser":     c.Locals("currentUser"),
		"StartDate":       startDate,
		"EndDate":         endDate,
		"SelectedMonth":   selectedMonth,
		"PeriodType":      periodType,
		"IsMultiDay":      startDate != endDate,
		"IsOffline":       summary.ErrorMessage != "",
		"HasData":         len(summary.Logs) > 0,
		"AvailableMonths": h.getAvailableMonths(selectedMonth),
		"CurrentYear":     time.Now().Year(),
	}, "layouts/main")
}

// RenderCetakPresensi menyajikan dokumen cetak formal PDF rekapitulasi kehadiran ASN
func (h *DashboardHandler) RenderCetakPresensi(c *fiber.Ctx) error {
	ctx := context.Background()
	startDate, endDate, periodLabel, selectedMonth, _ := h.parseFilterParams(c)

	var summary *models.AttendanceSummary
	if startDate == endDate {
		t, _ := time.Parse(time.DateOnly, startDate)
		summary = h.attendanceService.GetAttendanceByDate(ctx, t)
		// Jika hari ini tidak ada data (misal akhir pekan/libur) dan tidak ada query parameter spesifik,
		// tampilkan seluruh riwayat tahun berjalan agar dokumen cetak tidak kosong
		if summary.TotalPegawai == 0 && c.Query("period") == "" && c.Query("date") == "" && c.Query("month") == "" && c.Query("start_date") == "" {
			now := time.Now()
			startDate = fmt.Sprintf("%d-01-01", now.Year())
			endDate = now.Format(time.DateOnly)
			periodLabel = fmt.Sprintf("Semua Riwayat Log Presensi (Tahun %d)", now.Year())
			summary = h.attendanceService.GetAttendanceForPeriod(ctx, startDate, endDate)
		}
	} else {
		summary = h.attendanceService.GetAttendanceForPeriod(ctx, startDate, endDate)
	}

	if statusFilter := normalizeStatusFilter(c.Query("status")); statusFilter != statusFilterAll {
		// Salin summary agar cache data bersama tidak ikut terfilter.
		filtered := *summary
		filtered.Logs = filterLogsByStatus(summary.Logs, statusFilter)
		summary = &filtered
		periodLabel += " · Kategori: " + statusFilterLabel(statusFilter)
	}

	now := time.Now()
	printDateFormatted := fmt.Sprintf("%02d %s %d", now.Day(), getIndonesianMonth(now.Month()), now.Year())
	printTimeFormatted := now.Format("15:04:05")

	return c.Render("cetak-presensi", fiber.Map{
		"Title":              "Laporan Rekapitulasi Presensi ASN",
		"Summary":            summary,
		"DateDisplay":        periodLabel,
		"PrintDateFormatted": printDateFormatted,
		"PrintTimeFormatted": printTimeFormatted,
		"CurrentUser":        c.Locals("currentUser"),
		"SelectedMonth":      selectedMonth,
	})
}

// 2b. Mode TV / Display Lobby (tanpa sidebar & menu).
// Mendukung ?period=today|this_week|this_month|all untuk filter tabel log.
// Tidak pernah merender halaman error: saat BioTime mati total, tampilkan
// banner offline di atas data terakhir/kosong.
func (h *DashboardHandler) tvTodaySummary(ctx context.Context) (*models.AttendanceSummary, string) {
	now := time.Now()
	periodLabel := fmt.Sprintf("%s, %02d %s %d", services.IndonesianDayNames[now.Weekday()], now.Day(), getIndonesianMonth(now.Month()), now.Year())
	return h.attendanceService.GetAttendanceByDate(ctx, now), periodLabel
}

func (h *DashboardHandler) RenderTV(c *fiber.Ctx) error {
	ctx := context.Background()
	startDate, endDate, periodLabel, _, periodType := h.parseFilterParams(c)

	var summary *models.AttendanceSummary
	if startDate == endDate {
		t, _ := time.Parse(time.DateOnly, startDate)
		summary = h.attendanceService.GetAttendanceByDate(ctx, t)
	} else {
		summary = h.attendanceService.GetAttendanceForPeriod(ctx, startDate, endDate)
	}

	var activeRule *models.AttendanceRule
	if h.ruleService != nil {
		activeRule = h.ruleService.GetActiveRule()
	}

	return c.Render("tv", fiber.Map{
		"BioTimeVersion": config.GetEnv("BIOTIME_VERSION", "9.5"),
		"Title":          "Mode TV - MAPEL",
		"Summary":        summary,
		"Rule":           activeRule,
		"DateDisplay":    periodLabel,
		"IsOffline":      summary.ErrorMessage != "",
		"HasData":        len(summary.Logs) > 0,
		"PeriodType":     periodType,
		"IsMultiDay":     startDate != endDate,
		"CurrentYear":    time.Now().Year(),
	}, "layouts/tv")
}

func (h *DashboardHandler) RenderTVPresensi(c *fiber.Ctx) error {
	ctx := context.Background()
	summary, periodLabel := h.tvTodaySummary(ctx)

	return c.Render("tv-presensi", fiber.Map{
		"BioTimeVersion": config.GetEnv("BIOTIME_VERSION", "9.5"),
		"Title":          "Mode TV - Presensi ASN",
		"Summary":        summary,
		"DateDisplay":    periodLabel,
		"IsOffline":      summary.ErrorMessage != "",
		"HasData":        len(summary.Logs) > 0,
		"CurrentYear":    time.Now().Year(),
	}, "layouts/tv")
}

// 3. Dispensasi & Klarifikasi Handlers
func (h *DashboardHandler) RenderDispensasi(c *fiber.Ctx) error {
	ctx := context.Background()
	today := time.Now()
	dateStr := today.Format(time.DateOnly)

	// Fetch all employees from BioTime for the dropdown
	empResp, err := h.biotimeClient.FetchEmployees(ctx, 1, 1000, "", 0)
	if err != nil {
		return c.Render("error-500", fiber.Map{
			"ErrorMsg": "Gagal mengambil master data pegawai: " + err.Error(),
		})
	}
	employees := empResp.Data

	auditLogs := h.auditService.GetAllLogs()

	return c.Render("dispensasi", fiber.Map{
		"Title":          "Dispensasi & Klarifikasi Kehadiran ASN",
		"ActiveMenu":     "dispensasi",
		"CurrentUser":    c.Locals("currentUser"),
		"Employees":      employees,
		"DispensasiLogs": auditLogs,
		"TodayDate":      dateStr,
	}, "layouts/main")
}

// Deprecated fallback: RenderAuditBarisan redirect to /dispensasi
func (h *DashboardHandler) RenderAuditBarisan(c *fiber.Ctx) error {
	return c.Redirect("/dispensasi")
}

func (h *DashboardHandler) PostSaveDispensasi(c *fiber.Ctx) error {
	var req models.AuditVerifyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "Format request tidak valid",
		})
	}

	if req.EmpCode == "" || req.PhysicalStatus == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "Pegawai dan Jenis Dispensasi wajib diisi",
		})
	}

	if currentUser, ok := c.Locals("currentUser").(*models.User); ok && currentUser != nil {
		req.VerifierName = fmt.Sprintf("[ID: %d] %s (%s)", currentUser.ID, currentUser.FullName, currentUser.Role)
	}

	verifierIP := c.IP()
	record, err := h.auditService.RecordAudit(req, verifierIP)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"status":  "error",
			"message": "Gagal mencatat dispensasi: " + err.Error(),
		})
	}

	// Dispensasi mengubah status presensi: buang cache agar efeknya instan.
	h.attendanceService.InvalidateCache()

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Dispensasi kehadiran ASN berhasil disahkan",
		"data":    record,
	})
}

func (h *DashboardHandler) PostDeleteDispensasi(c *fiber.Ctx) error {
	var payload struct {
		ID string `json:"id"`
	}
	if err := c.BodyParser(&payload); err != nil || payload.ID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "ID dispensasi tidak valid",
		})
	}

	if err := h.auditService.DeleteAudit(payload.ID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"status":  "error",
			"message": "Gagal membatalkan dispensasi: " + err.Error(),
		})
	}

	h.attendanceService.InvalidateCache()

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Dispensasi berhasil dibatalkan",
	})
}

// Backward compatibility alias for PostAuditVerify
func (h *DashboardHandler) PostAuditVerify(c *fiber.Ctx) error {
	return h.PostSaveDispensasi(c)
}

func (h *DashboardHandler) GetAuditLogsJSON(c *fiber.Ctx) error {
	logs := h.auditService.GetAllLogs()
	return c.JSON(logs)
}

// 4. CRUD Petugas Handlers
func (h *DashboardHandler) RenderPetugas(c *fiber.Ctx) error {
	users := h.userService.GetAllUsers()
	roles := h.userService.GetAllRoles()
	return c.Render("petugas", fiber.Map{
		"Title":       "Manajemen Petugas Verifikator",
		"Users":       users,
		"Roles":       roles,
		"ActiveMenu":  "petugas",
		"CurrentUser": c.Locals("currentUser"),
	}, "layouts/main")
}

func (h *DashboardHandler) GetRolesJSON(c *fiber.Ctx) error {
	roles := h.userService.GetAllRolesWithCount()
	return c.JSON(fiber.Map{
		"status": "success",
		"data":   roles,
	})
}

func (h *DashboardHandler) PostCreateUser(c *fiber.Ctx) error {
	var req models.CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "Format payload tidak valid",
		})
	}

	newUser, err := h.userService.CreateUser(req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Akun petugas berhasil dibuat",
		"data":    newUser,
	})
}

func (h *DashboardHandler) PutUpdateUser(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "ID petugas tidak valid",
		})
	}

	var req models.UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "Format payload tidak valid",
		})
	}

	updated, err := h.userService.UpdateUser(id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Data petugas berhasil diperbarui",
		"data":    updated,
	})
}

func (h *DashboardHandler) DeleteUser(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "ID petugas tidak valid",
		})
	}

	if err := h.userService.DeleteUser(id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Akun petugas berhasil dihapus",
	})
}

// 5. CRUD Roles Handlers
func (h *DashboardHandler) RenderRoles(c *fiber.Ctx) error {
	roles := h.userService.GetAllRolesWithCount()
	return c.Render("roles", fiber.Map{
		"Title":       "Manajemen Hak Akses & Peran (Roles)",
		"Roles":       roles,
		"ActiveMenu":  "roles",
		"CurrentUser": c.Locals("currentUser"),
	}, "layouts/main")
}

func (h *DashboardHandler) GetRoleDetailJSON(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "ID role tidak valid",
		})
	}
	role, err := h.userService.GetRoleByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}
	return c.JSON(fiber.Map{
		"status": "success",
		"data":   role,
	})
}

func (h *DashboardHandler) PostCreateRole(c *fiber.Ctx) error {
	var req models.CreateRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "Format data tidak valid",
		})
	}
	newRole, err := h.userService.CreateRole(req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Role baru berhasil ditambahkan",
		"data":    newRole,
	})
}

func (h *DashboardHandler) PutUpdateRole(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "ID role tidak valid",
		})
	}
	var req models.UpdateRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "Format data tidak valid",
		})
	}
	updatedRole, err := h.userService.UpdateRole(id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Data role berhasil diperbarui",
		"data":    updatedRole,
	})
}

func (h *DashboardHandler) DeleteRole(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "ID role tidak valid",
		})
	}
	if err := h.userService.DeleteRole(id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Role berhasil dihapus",
	})
}

// 6. Informasi & Pengaturan Aturan Presensi
func (h *DashboardHandler) RenderAturanPresensi(c *fiber.Ctx) error {
	var rule *models.AttendanceRule
	if h.ruleService != nil {
		rule = h.ruleService.GetActiveRule()
	}
	return c.Render("aturan-presensi", fiber.Map{
		"Title":       "Matriks & Aturan Jam Kerja ASN",
		"ActiveMenu":  "aturan",
		"Rule":        rule,
		"CurrentUser": c.Locals("currentUser"),
	}, "layouts/main")
}

func (h *DashboardHandler) GetAttendanceRuleJSON(c *fiber.Ctx) error {
	if h.ruleService == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "error", "message": "RuleService tidak aktif"})
	}
	rule := h.ruleService.GetActiveRule()
	return c.JSON(fiber.Map{
		"status": "success",
		"data":   rule,
	})
}

func (h *DashboardHandler) PutAttendanceRule(c *fiber.Ctx) error {
	if h.ruleService == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "error", "message": "RuleService tidak aktif"})
	}
	var req models.UpdateAttendanceRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "Format payload tidak valid: " + err.Error(),
		})
	}
	updated, err := h.ruleService.UpdateRule(req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}
	// Aturan berubah: ringkasan ter-cache dihitung dengan aturan lama, buang.
	h.attendanceService.InvalidateCache()
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Aturan jam kerja ASN berhasil diperbarui",
		"data":    updated,
	})
}

func (h *DashboardHandler) GetSummaryJSON(c *fiber.Ctx) error {
	ctx := context.Background()
	dateParam := c.Query("date")
	if dateParam != "" {
		if t, err := time.Parse(time.DateOnly, dateParam); err == nil {
			return c.JSON(h.attendanceService.GetAttendanceByDate(ctx, t))
		}
	}
	startDate, endDate, _, _, _ := h.parseFilterParams(c)
	var summary *models.AttendanceSummary
	if startDate == endDate {
		t, err := time.Parse(time.DateOnly, startDate)
		if err == nil {
			summary = h.attendanceService.GetAttendanceByDate(ctx, t)
		} else {
			summary = h.attendanceService.GetTodayAttendance(ctx)
		}
	} else {
		summary = h.attendanceService.GetAttendanceForPeriod(ctx, startDate, endDate)
	}
	return c.JSON(summary)
}

func (h *DashboardHandler) GetDistributionJSON(c *fiber.Ctx) error {
	ctx := context.Background()
	startDate, endDate, _, _, _ := h.parseFilterParams(c)
	var summary *models.AttendanceSummary
	if startDate == endDate {
		t, err := time.Parse(time.DateOnly, startDate)
		if err == nil {
			summary = h.attendanceService.GetAttendanceByDate(ctx, t)
		} else {
			summary = h.attendanceService.GetTodayAttendance(ctx)
		}
	} else {
		summary = h.attendanceService.GetAttendanceForPeriod(ctx, startDate, endDate)
	}

	dist := models.ChartDistribution{
		TepatWaktu: summary.TepatWaktuCount,
		Terlambat:  summary.TerlambatCount,
		ApelSore:   summary.ApelSoreCount,
		Anomali:    summary.AnomaliCount,
		Series:     []int{summary.TepatWaktuCount, summary.TerlambatCount, summary.ApelSoreCount, summary.AnomaliCount},
		Labels:     []string{"Tepat Waktu (Pagi)", "Terlambat (Pagi)", "Hadir Apel Sore", "Temuan Ketidaksesuaian"},
	}
	return c.JSON(dist)
}

func (h *DashboardHandler) GetChartDataJSON(c *fiber.Ctx) error {
	ctx := context.Background()
	period := c.Query("period", "weekly")
	chartData := h.attendanceService.GetChartData(ctx, period)
	return c.JSON(chartData)
}

func getIndonesianMonth(m time.Month) string {
	months := [...]string{
		"Januari", "Februari", "Maret", "April", "Mei", "Juni",
		"Juli", "Agustus", "September", "Oktober", "November", "Desember",
	}
	if m >= 1 && m <= 12 {
		return months[m-1]
	}
	return ""
}
