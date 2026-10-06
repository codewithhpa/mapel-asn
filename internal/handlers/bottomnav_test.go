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

func setupBottomNavTestApp(t *testing.T) (*fiber.App, *services.UserService) {
	t.Helper()
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/jwt-api-token-auth/" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "navmock"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"count":0,"code":0,"data":[]}`))
	}))
	t.Cleanup(mock.Close)
	t.Setenv("BIOTIME_BASE_URL", mock.URL)

	engine := html.New("../../web/views", ".html")
	engine.AddFunc("add", func(a, b int) int { return a + b })
	engine.AddFunc("isLeave", models.IsExcusedLeaveStatus)
	app := fiber.New(fiber.Config{Views: engine})
	client := services.NewBioTimeClient()
	svc := services.NewAttendanceService(client, services.NewAttendanceEngine(), services.NewAttendanceCache())
	dir := t.TempDir()
	auditSvc := services.NewAuditService(filepath.Join(dir, "a.json"), nil)
	userSvc := services.NewUserService(filepath.Join(dir, "u.json"), nil)
	ruleSvc := services.NewRuleService(nil)
	SetupRoutes(app, svc, auditSvc, userSvc, ruleSvc)
	return app, userSvc
}

func loginAs(t *testing.T, app *fiber.App, username, password string) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(models.UserLoginRequest{Username: username, Password: password})
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("login %s gagal: %v status %d", username, err, resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == "auth_session" {
			return c
		}
	}
	t.Fatal("cookie auth_session tidak ada")
	return nil
}

func getBody(t *testing.T, app *fiber.App, path string, cookie *http.Cookie) string {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("GET %s want 200, got %d", path, resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestBottomNav_AdminSeesFiveItems(t *testing.T) {
	app, _ := setupBottomNavTestApp(t)
	adminCookie := loginAs(t, app, "admin", "adminkey1234")
	body := getBody(t, app, "/", adminCookie)

	for _, want := range []string{
		`mobile-bottom-nav`,
		`href="/"`,
		`href="/presensi"`,
		`href="/dispensasi"`,
		`href="/aturan-presensi"`,
		`bottom-nav-item active`, // Beranda aktif di dashboard
	} {
		if !strings.Contains(body, want) {
			t.Errorf("admin nav harus memuat %q", want)
		}
	}
}

func TestBottomNav_PetugasNoAturan(t *testing.T) {
	app, userSvc := setupBottomNavTestApp(t)
	if _, err := userSvc.CreateUser(models.CreateUserRequest{
		Username: "piket1", Password: "piket1234", FullName: "Petugas Piket", RoleID: 3,
	}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cookie := loginAs(t, app, "piket1", "piket1234")
	body := getBody(t, app, "/", cookie)

	if !strings.Contains(body, `href="/dispensasi"`) {
		t.Error("petugas harus melihat menu Dispensasi")
	}
	if strings.Contains(body, `href="/aturan-presensi"`) {
		t.Error("petugas non-admin TIDAK boleh melihat menu Aturan di bottom nav")
	}
}

func TestBottomNav_PublicSeesAturan(t *testing.T) {
	app, _ := setupBottomNavTestApp(t)
	body := getBody(t, app, "/", nil)

	if !strings.Contains(body, `href="/aturan-presensi"`) {
		t.Error("publik harus melihat menu Aturan")
	}
	if strings.Contains(body, `href="/dispensasi"`) {
		t.Error("publik TIDAK boleh melihat menu Dispensasi")
	}
}
