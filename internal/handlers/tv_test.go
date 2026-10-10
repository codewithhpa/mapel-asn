package handlers

import (
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

func getTVBody(t *testing.T, app *fiber.App, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	resp, err := app.Test(req, 15000)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestTV_DashboardRendersWithoutChrome(t *testing.T) {
	app, _ := setupBottomNavTestApp(t)

	code, body := getTVBody(t, app, "/tv")
	if code != 200 {
		t.Fatalf("GET /tv want 200, got %d", code)
	}
	for _, want := range []string{
		`id="tvClock"`, `id="tvRefreshSelect"`, `id="chart-profile-visit`,
		`id="chart-visitors-profile"`, `Tingkat Kepatuhan Apel`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/tv harus memuat %q", want)
		}
	}
	for _, banned := range []string{`id="sidebar"`, `mobile-bottom-nav`, `userProfileDropdown`} {
		if strings.Contains(body, banned) {
			t.Errorf("/tv tidak boleh memuat chrome operator %q", banned)
		}
	}
}

func TestTV_LogPeriodFilter(t *testing.T) {
	app, _ := setupBottomNavTestApp(t)

	for _, tc := range []struct {
		period string
		active string
	}{
		{"today", `<a href="/tv?period=today" class="btn active">`},
		{"this_week", `<a href="/tv?period=this_week" class="btn active">`},
		{"this_month", `<a href="/tv?period=this_month" class="btn active">`},
		{"all", `<a href="/tv?period=all" class="btn active">`},
	} {
		code, body := getTVBody(t, app, "/tv?period="+tc.period)
		if code != 200 {
			t.Fatalf("GET /tv?period=%s want 200, got %d", tc.period, code)
		}
		if !strings.Contains(body, tc.active) {
			t.Errorf("/tv?period=%s harus menandai tombol aktif %q", tc.period, tc.active)
		}
		for _, want := range []string{"Minggu Ini", "Bulan Ini", "Tahunan", "Data Realtime"} {
			if !strings.Contains(body, want) {
				t.Errorf("/tv?period=%s harus memuat %q", tc.period, want)
			}
		}
	}
}

func TestTV_PresensiRendersTable(t *testing.T) {
	app, _ := setupBottomNavTestApp(t)

	code, body := getTVBody(t, app, "/tv/presensi")
	if code != 200 {
		t.Fatalf("GET /tv/presensi want 200, got %d", code)
	}
	for _, want := range []string{`id="tvPresensiTable"`, `id="tvRefreshSelect"`, `Presensi ASN Hari Ini`} {
		if !strings.Contains(body, want) {
			t.Errorf("/tv/presensi harus memuat %q", want)
		}
	}
	if strings.Contains(body, `id="sidebar"`) {
		t.Error("/tv/presensi tidak boleh memuat sidebar")
	}
}

func TestTV_OfflineShowsBannerNotErrorPage(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/jwt-api-token-auth/" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "tvdown"})
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

	for _, path := range []string{"/tv", "/tv/presensi"} {
		code, body := getTVBody(t, app, path)
		if code != 200 {
			t.Fatalf("GET %s saat offline want 200 (banner, bukan error page), got %d", path, code)
		}
		if !strings.Contains(body, "terputus") {
			t.Errorf("GET %s harus menampilkan banner offline", path)
		}
	}
}
