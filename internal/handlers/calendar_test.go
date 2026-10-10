package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"dashboard-presensi/internal/models"
	"dashboard-presensi/internal/services"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
)

// setupCalendarTestApp app handler test lengkap dengan kalender kedinasan aktif (fallback in-memory).
func setupCalendarTestApp(t *testing.T) (*fiber.App, *http.Cookie, *services.CalendarService) {
	t.Helper()

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/jwt-api-token-auth/" {
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "mock_jwt"})
			return
		}
		if r.URL.Path == "/iclock/api/terminals/" {
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 0, "code": 0, "data": []any{}})
			return
		}
		if r.URL.Path == "/iclock/api/transactions/" {
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 0, "code": 0, "data": []any{}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(mock.Close)
	t.Setenv("BIOTIME_BASE_URL", mock.URL)
	t.Setenv("BIOTIME_USERNAME", "admin")
	t.Setenv("BIOTIME_PASSWORD", "password")
	t.Setenv("LOGIN_RATE_LIMIT", "1000")

	engine := html.New("../../web/views", ".html")
	engine.Reload(true)
	engine.AddFunc("add", func(a, b int) int { return a + b })
	engine.AddFunc("mul", func(a, b int) int { return a * b })
	engine.AddFunc("isLeave", models.IsExcusedLeaveStatus)
	engine.AddFunc("calendarTypeBadge", CalendarTypeBadgeFunc)
	engine.AddFunc("calendarHoursLabel", CalendarHoursLabelFunc)
	engine.AddFunc("shortDateID", ShortDateIDFunc)
	engine.AddFunc("shortRangeID", ShortRangeIDFunc)
	engine.AddFunc("calendarHoursChips", CalendarHoursChipsFunc)

	app := fiber.New(fiber.Config{Views: engine})
	client := services.NewBioTimeClient()
	attSvc := services.NewAttendanceService(client, services.NewAttendanceEngine(), services.NewAttendanceCache())
	calSvc := services.NewCalendarService(nil)
	attSvc.SetCalendarService(calSvc)
	auditSvc := services.NewAuditService(filepath.Join(t.TempDir(), "audit.json"), nil)
	userSvc := services.NewUserService(filepath.Join(t.TempDir(), "users.json"), nil)
	SetupRoutes(app, attSvc, auditSvc, userSvc, services.NewRuleService(nil))

	loginJSON, _ := json.Marshal(models.UserLoginRequest{Username: "admin", Password: "adminkey1234"})
	reqLogin := httptest.NewRequest("POST", "/login", bytes.NewReader(loginJSON))
	reqLogin.Header.Set("Content-Type", "application/json")
	respLogin, err := app.Test(reqLogin, 5000)
	if err != nil || respLogin.StatusCode != 200 {
		t.Fatalf("login gagal: %v status=%v", err, respLogin.StatusCode)
	}
	for _, c := range respLogin.Cookies() {
		if c.Name == "auth_session" {
			return app, c, calSvc
		}
	}
	t.Fatal("cookie auth_session tidak ada")
	return nil, nil, nil
}

func TestCalendarCRUD_AdminOnly(t *testing.T) {
	app, cookie, calSvc := setupCalendarTestApp(t)

	// Tanpa login ditolak
	respNo := httptest.NewRequest("GET", "/api/calendar", nil)
	if r, err := app.Test(respNo, 5000); err != nil || r.StatusCode == 200 {
		t.Fatalf("/api/calendar tanpa login harus ditolak, dapat %v status=%v", err, r.StatusCode)
	}

	// Create
	payload, _ := json.Marshal(models.CalendarEventRequest{
		Type: "cuti_bersama", Name: "Cuti Bersama Idulfitri",
		StartDate: "2026-03-20", EndDate: "2026-03-20",
	})
	reqC := httptest.NewRequest("POST", "/api/calendar", bytes.NewReader(payload))
	reqC.Header.Set("Content-Type", "application/json")
	reqC.AddCookie(cookie)
	respC, err := app.Test(reqC, 5000)
	if err != nil || respC.StatusCode != 201 {
		t.Fatalf("create gagal: %v status=%v", err, respC.StatusCode)
	}

	// List berisi event
	reqL := httptest.NewRequest("GET", "/api/calendar?year=2026", nil)
	reqL.AddCookie(cookie)
	respL, err := app.Test(reqL, 5000)
	if err != nil || respL.StatusCode != 200 {
		t.Fatalf("list gagal: %v", err)
	}
	bodyL, _ := io.ReadAll(respL.Body)
	if !strings.Contains(string(bodyL), "Cuti Bersama Idulfitri") {
		t.Fatalf("list harus memuat event baru: %s", bodyL)
	}

	// Update
	events, _ := calSvc.ListEvents(2026)
	if len(events) == 0 {
		t.Fatal("kalender kosong setelah create")
	}
	upPayload, _ := json.Marshal(models.CalendarEventRequest{
		Type: "libur_nasional", Name: "Idulfitri 1447 H",
		StartDate: "2026-03-21", EndDate: "2026-03-22",
	})
	reqU := httptest.NewRequest("PUT", "/api/calendar/1", bytes.NewReader(upPayload))
	reqU.Header.Set("Content-Type", "application/json")
	reqU.AddCookie(cookie)
	respU, err := app.Test(reqU, 5000)
	if err != nil || respU.StatusCode != 200 {
		t.Fatalf("update gagal: %v status=%v", err, respU.StatusCode)
	}

	// Delete
	reqD := httptest.NewRequest("DELETE", "/api/calendar/1", nil)
	reqD.AddCookie(cookie)
	respD, err := app.Test(reqD, 5000)
	if err != nil || respD.StatusCode != 200 {
		t.Fatalf("delete gagal: %v status=%v", err, respD.StatusCode)
	}
}

func TestAturanPresensi_RendersCalendarSection(t *testing.T) {
	app, cookie, calSvc := setupCalendarTestApp(t)

	_, err := calSvc.CreateEvent(models.CalendarEventRequest{
		Type: "libur_nasional", Name: "Hari Suci Nyepi (Tahun Baru Saka)",
		StartDate: "2026-03-19", EndDate: "2026-03-19",
	}, "Admin Tes")
	if err != nil {
		t.Fatalf("setup event gagal: %v", err)
	}

	req := httptest.NewRequest("GET", "/aturan-presensi", nil)
	req.AddCookie(cookie)
	resp, err := app.Test(req, 10000)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("aturan-presensi gagal: %v status=%v", err, resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	for _, want := range []string{
		"Kalender Kedinasan", "Hari Suci Nyepi", "19 Mar 2026",
		"Tambah Event", "Jam Kerja Khusus",
		">Libur Nasional<", "loadCalendar",
		`id="sec-matriks"`, `id="sec-simulator"`, `id="sec-kebijakan"`, `id="sec-kalender"`,
		`id="calSearch"`, `id="calTypeFilter"`, `cal-hour-chip`, `<details`,
		`shortRangeID`, `calHoursChips`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("aturan-presensi harus memuat %q", want)
		}
	}
}

func TestShortDateIDHelpers(t *testing.T) {
	if got := ShortDateIDFunc("2026-03-21"); got != "21 Mar 2026" {
		t.Errorf("ShortDateID = %q", got)
	}
	if got := ShortDateIDFunc("2026-08-17"); got != "17 Agu 2026" {
		t.Errorf("ShortDateID Agu = %q", got)
	}
	if got := ShortRangeIDFunc("2026-03-21", "2026-03-21"); got != "21 Mar 2026" {
		t.Errorf("ShortRange sama = %q", got)
	}
	if got := ShortRangeIDFunc("2026-03-21", "2026-03-22"); got != "21-22 Mar 2026" {
		t.Errorf("ShortRange sebulan = %q", got)
	}
	if got := ShortRangeIDFunc("2026-12-30", "2027-01-02"); got != "30 Des 2026 - 2 Jan 2027" {
		t.Errorf("ShortRange beda = %q", got)
	}
	if got := ShortRangeIDFunc("2026-03-21", ""); got != "21 Mar 2026" {
		t.Errorf("ShortRange end kosong = %q", got)
	}
}

// TestCalendarNilService_NoPanic regresi insiden 502: service kalender belum
// di-wire (nil) harus dibalas 503, bukan panic yang mematikan server.
func TestCalendarNilService_NoPanic(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/jwt-api-token-auth/" {
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "mock_jwt"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mock.Close()
	t.Setenv("BIOTIME_BASE_URL", mock.URL)
	t.Setenv("BIOTIME_USERNAME", "admin")
	t.Setenv("BIOTIME_PASSWORD", "password")
	t.Setenv("LOGIN_RATE_LIMIT", "1000")

	engine := html.New("../../web/views", ".html")
	engine.Reload(true)
	engine.AddFunc("add", func(a, b int) int { return a + b })
	engine.AddFunc("mul", func(a, b int) int { return a * b })
	engine.AddFunc("isLeave", models.IsExcusedLeaveStatus)
	engine.AddFunc("calendarTypeBadge", CalendarTypeBadgeFunc)
	engine.AddFunc("calendarHoursLabel", CalendarHoursLabelFunc)
	engine.AddFunc("shortDateID", ShortDateIDFunc)
	engine.AddFunc("shortRangeID", ShortRangeIDFunc)
	engine.AddFunc("calendarHoursChips", CalendarHoursChipsFunc)

	app := fiber.New(fiber.Config{Views: engine})
	client := services.NewBioTimeClient()
	attSvc := services.NewAttendanceService(client, services.NewAttendanceEngine(), services.NewAttendanceCache())
	// SENGAJA tanpa SetCalendarService -> CalendarService() nil.
	auditSvc := services.NewAuditService(filepath.Join(t.TempDir(), "audit.json"), nil)
	userSvc := services.NewUserService(filepath.Join(t.TempDir(), "users.json"), nil)
	SetupRoutes(app, attSvc, auditSvc, userSvc, services.NewRuleService(nil))

	loginJSON, _ := json.Marshal(models.UserLoginRequest{Username: "admin", Password: "adminkey1234"})
	reqLogin := httptest.NewRequest("POST", "/login", bytes.NewReader(loginJSON))
	reqLogin.Header.Set("Content-Type", "application/json")
	respLogin, err := app.Test(reqLogin, 5000)
	if err != nil || respLogin.StatusCode != 200 {
		t.Fatalf("login gagal: %v", err)
	}
	var cookie *http.Cookie
	for _, c := range respLogin.Cookies() {
		if c.Name == "auth_session" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("tanpa cookie")
	}
	req := httptest.NewRequest("GET", "/api/calendar?year=2026", nil)
	req.AddCookie(cookie)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("request tidak boleh panic/error transport: %v", err)
	}
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("tanpa calendar service harus 503, dapat %d", resp.StatusCode)
	}
}
