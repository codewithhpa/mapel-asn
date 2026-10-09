package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"dashboard-presensi/internal/models"
	"dashboard-presensi/internal/services"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
)

func setupAuthTestApp(t *testing.T) *fiber.App {
	t.Helper()
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/jwt-api-token-auth/" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "x"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"count":0,"code":0,"data":[]}`))
	}))
	t.Cleanup(mock.Close)
	t.Setenv("BIOTIME_BASE_URL", mock.URL)

	engine := html.New("../../web/views", ".html")
	engine.AddFunc("add", func(a, b int) int { return a + b })
	engine.AddFunc("mul", func(a, b int) int { return a * b })
	engine.AddFunc("isLeave", models.IsExcusedLeaveStatus)
	app := fiber.New(fiber.Config{Views: engine})
	client := services.NewBioTimeClient()
	svc := services.NewAttendanceService(client, services.NewAttendanceEngine(), services.NewAttendanceCache())
	dir := t.TempDir()
	auditSvc := services.NewAuditService(filepath.Join(dir, "a.json"), nil)
	userSvc := services.NewUserService(filepath.Join(dir, "u.json"), nil)
	ruleSvc := services.NewRuleService(nil)
	SetupRoutes(app, svc, auditSvc, userSvc, ruleSvc)
	return app
}

func loginAndCookie(t *testing.T, app *fiber.App) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(models.UserLoginRequest{Username: "admin", Password: "adminkey1234"})
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("login gagal: %v status %d", err, resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == "auth_session" {
			return c
		}
	}
	t.Fatal("cookie auth_session tidak ada")
	return nil
}

func TestAuth_OpaqueToken_RejectsForgedID(t *testing.T) {
	app := setupAuthTestApp(t)
	cookie := loginAndCookie(t, app)
	if cookie.Value == "1" {
		t.Fatal("cookie masih ID polos, harus token opaque")
	}

	// Token valid -> 200
	req := httptest.NewRequest("GET", "/dispensasi", nil)
	req.AddCookie(cookie)
	resp, _ := app.Test(req, 5000)
	if resp.StatusCode != 200 {
		t.Fatalf("token valid harus 200, got %d", resp.StatusCode)
	}

	// Cookie palsu "1" (eksploitasi lama) -> wajib ditolak 302
	forged := httptest.NewRequest("GET", "/dispensasi", nil)
	forged.AddCookie(&http.Cookie{Name: "auth_session", Value: "1"})
	respForged, _ := app.Test(forged, 5000)
	if respForged.StatusCode != 302 {
		t.Fatalf("cookie palsu harus 302 ke /login, got %d", respForged.StatusCode)
	}

	// Logout mencabut sesi -> token lama ditolak
	reqOut := httptest.NewRequest("GET", "/logout", nil)
	reqOut.AddCookie(cookie)
	_, _ = app.Test(reqOut, 5000)
	reqReuse := httptest.NewRequest("GET", "/dispensasi", nil)
	reqReuse.AddCookie(cookie)
	respReuse, _ := app.Test(reqReuse, 5000)
	if respReuse.StatusCode != 302 {
		t.Fatalf("token setelah logout harus 302, got %d", respReuse.StatusCode)
	}
}

func TestAuth_LoginRateLimit(t *testing.T) {
	ip := "10.99.99.99"
	for i := 0; i < 10; i++ {
		if !loginAllowed(ip) {
			t.Fatalf("percobaan %d seharusnya diizinkan", i+1)
		}
	}
	if loginAllowed(ip) {
		t.Fatal("percobaan ke-11 dalam 1 menit harus ditolak (429)")
	}
}
