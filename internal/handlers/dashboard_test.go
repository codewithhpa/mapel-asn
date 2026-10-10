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

func TestRenderDashboardPresensiAndAuth(t *testing.T) {
	// Mock BioTime agar test tidak bergantung ke server live 127.0.0.1:8081.
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/jwt-api-token-auth/" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "mock_jwt"})
			return
		}
		if r.URL.Path == "/iclock/api/transactions/" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 0, "code": 0, "data": []any{}})
			return
		}
		if r.URL.Path == "/personnel/api/employees/" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 0, "data": []any{}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mock.Close()
	t.Setenv("BIOTIME_BASE_URL", mock.URL)
	t.Setenv("BIOTIME_USERNAME", "admin")
	t.Setenv("BIOTIME_PASSWORD", "password")
	t.Setenv("LOGIN_RATE_LIMIT", "1000")

	// Setup template engine pointing to views
	engine := html.New("../../web/views", ".html")
	engine.Reload(true)
	engine.AddFunc("add", func(a, b int) int {
		return a + b
	})
	engine.AddFunc("mul", func(a, b int) int {
		return a * b
	})
	engine.AddFunc("isLeave", models.IsExcusedLeaveStatus)
	engine.AddFunc("calendarTypeBadge", CalendarTypeBadgeFunc)
	engine.AddFunc("calendarHoursLabel", CalendarHoursLabelFunc)
	engine.AddFunc("shortDateID", ShortDateIDFunc)
	engine.AddFunc("shortRangeID", ShortRangeIDFunc)
	engine.AddFunc("calendarHoursChips", CalendarHoursChipsFunc)

	app := fiber.New(fiber.Config{
		Views: engine,
	})

	client := services.NewBioTimeClient()
	attendanceEngine := services.NewAttendanceEngine()
	cache := services.NewAttendanceCache()
	service := services.NewAttendanceService(client, attendanceEngine, cache)

	tempDir := t.TempDir()
	auditPath := filepath.Join(tempDir, "test_audit_handler.json")
	auditSvc := services.NewAuditService(auditPath, nil)

	userPath := filepath.Join(tempDir, "test_user_handler.json")
	userSvc := services.NewUserService(userPath, nil)
	ruleSvc := services.NewRuleService(nil)

	SetupRoutes(app, service, auditSvc, userSvc, ruleSvc)

	// 1. Test GET / (Dashboard - Publik)
	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatalf("Failed to execute request GET /: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200 on /, got %d", resp.StatusCode)
	}

	// 2. Test GET /dispensasi Tanpa Login -> Wajib Redirect ke /login (302)
	reqUnauth := httptest.NewRequest("GET", "/dispensasi", nil)
	respUnauth, err := app.Test(reqUnauth, 5000)
	if err != nil {
		t.Fatalf("Failed unauth test: %v", err)
	}
	if respUnauth.StatusCode != 302 {
		t.Errorf("Expected status 302 redirect for unauthenticated access, got %d", respUnauth.StatusCode)
	}

	// 3. Test POST /login (Login Berhasil dengan admin / adminkey1234)
	loginPayload := models.UserLoginRequest{
		Username: "admin",
		Password: "adminkey1234",
	}
	loginJSON, _ := json.Marshal(loginPayload)
	reqLogin := httptest.NewRequest("POST", "/login", bytes.NewReader(loginJSON))
	reqLogin.Header.Set("Content-Type", "application/json")
	respLogin, err := app.Test(reqLogin, 5000)
	if err != nil {
		t.Fatalf("Failed login test: %v", err)
	}
	if respLogin.StatusCode != 200 {
		t.Errorf("Expected status 200 on /login, got %d", respLogin.StatusCode)
	}

	// Ambil cookie auth_session
	var sessionCookie *http.Cookie
	for _, c := range respLogin.Cookies() {
		if c.Name == "auth_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("Expected auth_session cookie to be set")
	}

	// 4. Test GET /dispensasi Dengan Cookie -> Berhasil (200 OK)
	reqAuth := httptest.NewRequest("GET", "/dispensasi", nil)
	reqAuth.AddCookie(sessionCookie)
	respAuth, err := app.Test(reqAuth, 5000)
	if err != nil {
		t.Fatalf("Failed auth request: %v", err)
	}
	if respAuth.StatusCode != 200 {
		t.Errorf("Expected status 200 on /dispensasi with session, got %d", respAuth.StatusCode)
	}

	// 5. Test GET /petugas Dengan Cookie -> Berhasil (200 OK)
	reqPetugas := httptest.NewRequest("GET", "/petugas", nil)
	reqPetugas.AddCookie(sessionCookie)
	respPetugas, err := app.Test(reqPetugas, 5000)
	if err != nil {
		t.Fatalf("Failed /petugas request: %v", err)
	}
	if respPetugas.StatusCode != 200 {
		t.Errorf("Expected status 200 on /petugas, got %d", respPetugas.StatusCode)
	}
	bodyPetugas, _ := io.ReadAll(respPetugas.Body)
	if !strings.Contains(string(bodyPetugas), "Manajemen Petugas") {
		t.Errorf("Expected body to contain 'Manajemen Petugas'")
	}
}

// Dashboard utama ikut pola TV: BioTime mati total bukan error page —
// banner offline + data terakhir (atau empty state), tetap 200.
func TestDashboard_OfflineShowsBannerNotErrorPage(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/jwt-api-token-auth/" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "down"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"detail":"mati"}`))
	}))
	defer mock.Close()
	t.Setenv("BIOTIME_BASE_URL", mock.URL)
	t.Setenv("LOGIN_RATE_LIMIT", "1000")

	engine := html.New("../../web/views", ".html")
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
	svc := services.NewAttendanceService(client, services.NewAttendanceEngine(), services.NewAttendanceCache())
	dir := t.TempDir()
	auditSvc := services.NewAuditService(filepath.Join(dir, "a.json"), nil)
	userSvc := services.NewUserService(filepath.Join(dir, "u.json"), nil)
	SetupRoutes(app, svc, auditSvc, userSvc, services.NewRuleService(nil))

	for _, tc := range []struct {
		path string
		want string
	}{
		{"/", "MAPEL"},
		{"/presensi", "Daftar Rekapitulasi"},
	} {
		req := httptest.NewRequest("GET", tc.path, nil)
		resp, err := app.Test(req, 15000)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		b, _ := io.ReadAll(resp.Body)
		body := string(b)
		if resp.StatusCode != 200 {
			t.Errorf("GET %s saat offline want 200 (banner, bukan error page), got %d", tc.path, resp.StatusCode)
		}
		if !strings.Contains(body, "terputus") {
			t.Errorf("GET %s harus menampilkan banner offline", tc.path)
		}
		if !strings.Contains(body, tc.want) {
			t.Errorf("GET %s harus tetap merender halaman (%q)", tc.path, tc.want)
		}
		if strings.Contains(body, "Detail Error Internal") {
			t.Errorf("GET %s tidak boleh menampilkan halaman error merah", tc.path)
		}
	}
}
