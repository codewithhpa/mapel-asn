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

func setupDeviceTestApp(t *testing.T, bodies map[string][]byte) (*fiber.App, *http.Cookie) {
	t.Helper()

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/jwt-api-token-auth/":
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "mock_jwt"})
		case "/iclock/api/terminals/":
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 1, "code": 0, "data": []any{
				map[string]any{"id": 4, "sn": "GAT5261100070", "alias": "SpeedFace M4", "terminal_name": "SpeedFace M4", "ip_address": "192.168.1.10", "area_name": "DISKOMINFO-Server", "state": "1", "last_activity": "2026-10-09 13:05:25", "user_count": 9, "face_count": 6, "is_attendance": 1, "transfer_interval": 1},
			}})
		case "/iclock/api/terminals/upload_transaction/":
			raw, _ := io.ReadAll(r.Body)
			bodies["upload"] = raw
			_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Success."})
		case "/personnel/api/employees/resync_to_device/":
			raw, _ := io.ReadAll(r.Body)
			bodies["resync"] = raw
			_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Success."})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(mock.Close)
	t.Setenv("BIOTIME_BASE_URL", mock.URL)
	t.Setenv("BIOTIME_USERNAME", "admin")
	t.Setenv("BIOTIME_PASSWORD", "password")

	engine := html.New("../../web/views", ".html")
	engine.Reload(true)
	engine.AddFunc("add", func(a, b int) int { return a + b })
	engine.AddFunc("mul", func(a, b int) int { return a * b })
	engine.AddFunc("isLeave", models.IsExcusedLeaveStatus)

	app := fiber.New(fiber.Config{Views: engine})
	client := services.NewBioTimeClient()
	service := services.NewAttendanceService(client, services.NewAttendanceEngine(), services.NewAttendanceCache())
	auditSvc := services.NewAuditService(filepath.Join(t.TempDir(), "audit.json"), nil)
	userSvc := services.NewUserService(filepath.Join(t.TempDir(), "users.json"), nil)
	SetupRoutes(app, service, auditSvc, userSvc, services.NewRuleService(nil))

	loginJSON, _ := json.Marshal(models.UserLoginRequest{Username: "admin", Password: "adminkey1234"})
	reqLogin := httptest.NewRequest("POST", "/login", bytes.NewReader(loginJSON))
	reqLogin.Header.Set("Content-Type", "application/json")
	respLogin, err := app.Test(reqLogin, 5000)
	if err != nil || respLogin.StatusCode != 200 {
		t.Fatalf("login gagal: %v status=%v", err, respLogin.StatusCode)
	}
	for _, c := range respLogin.Cookies() {
		if c.Name == "auth_session" {
			return app, c
		}
	}
	t.Fatal("cookie auth_session tidak ada")
	return nil, nil
}

func TestDevicePage_RendersWithAdmin(t *testing.T) {
	app, cookie := setupDeviceTestApp(t, map[string][]byte{})

	req := httptest.NewRequest("GET", "/device", nil)
	req.AddCookie(cookie)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatalf("GET /device gagal: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("mau 200, dapat %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	for _, want := range []string{"Kelola Device", "SpeedFace M4", "GAT5261100070", "Tarik Transaksi", "Sync Pegawai ke Device"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("body harus memuat %q", want)
		}
	}
}

func TestDevicePage_RequiresAdmin(t *testing.T) {
	app, _ := setupDeviceTestApp(t, map[string][]byte{})

	req := httptest.NewRequest("GET", "/device", nil)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("request gagal: %v", err)
	}
	if resp.StatusCode == 200 {
		t.Errorf("/device tanpa login harus ditolak, dapat 200")
	}
}

func TestDeviceAPI_UploadAndResync(t *testing.T) {
	bodies := map[string][]byte{}
	app, cookie := setupDeviceTestApp(t, bodies)

	// Upload per device via param
	reqUp := httptest.NewRequest("POST", "/api/device/4/upload", nil)
	reqUp.AddCookie(cookie)
	respUp, err := app.Test(reqUp, 5000)
	if err != nil || respUp.StatusCode != 200 {
		t.Fatalf("upload gagal: %v status=%v", err, respUp.StatusCode)
	}
	var upBody struct {
		Terminals []int64 `json:"terminals"`
	}
	if err := json.Unmarshal(bodies["upload"], &upBody); err != nil || len(upBody.Terminals) != 1 || upBody.Terminals[0] != 4 {
		t.Fatalf("body upload tidak diteruskan benar: %s", bodies["upload"])
	}

	// Resync pegawai
	payload, _ := json.Marshal(map[string]any{"employee_ids": []int64{11, 22}})
	reqRe := httptest.NewRequest("POST", "/api/device/resync", bytes.NewReader(payload))
	reqRe.Header.Set("Content-Type", "application/json")
	reqRe.AddCookie(cookie)
	respRe, err := app.Test(reqRe, 5000)
	if err != nil || respRe.StatusCode != 200 {
		t.Fatalf("resync gagal: %v status=%v", err, respRe.StatusCode)
	}
	var reBody struct {
		Employees []int64 `json:"employees"`
	}
	if err := json.Unmarshal(bodies["resync"], &reBody); err != nil || len(reBody.Employees) != 2 {
		t.Fatalf("body resync tidak diteruskan benar: %s", bodies["resync"])
	}

	// Resync kosong ditolak
	reqEmpty := httptest.NewRequest("POST", "/api/device/resync", bytes.NewReader([]byte(`{"employee_ids":[]}`)))
	reqEmpty.Header.Set("Content-Type", "application/json")
	reqEmpty.AddCookie(cookie)
	respEmpty, err := app.Test(reqEmpty, 5000)
	if err != nil || respEmpty.StatusCode != 400 {
		t.Fatalf("resync kosong harus 400, dapat %v status=%v", err, respEmpty.StatusCode)
	}
}

func TestBioTimeStatus_OnlineAndOffline(t *testing.T) {
	newStatusApp := func(t *testing.T, terminalsOK bool) *fiber.App {
		t.Helper()
		mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/jwt-api-token-auth/" {
				_ = json.NewEncoder(w).Encode(map[string]string{"token": "mock_jwt"})
				return
			}
			if r.URL.Path == "/iclock/api/terminals/" {
				if !terminalsOK {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"count": 1, "code": 0, "data": []any{}})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(mock.Close)
		t.Setenv("BIOTIME_BASE_URL", mock.URL)
		t.Setenv("BIOTIME_USERNAME", "admin")
		t.Setenv("BIOTIME_PASSWORD", "password")

		engine := html.New("../../web/views", ".html")
		engine.Reload(true)
		engine.AddFunc("add", func(a, b int) int { return a + b })
		engine.AddFunc("mul", func(a, b int) int { return a * b })
		engine.AddFunc("isLeave", models.IsExcusedLeaveStatus)

		app := fiber.New(fiber.Config{Views: engine})
		client := services.NewBioTimeClient()
		service := services.NewAttendanceService(client, services.NewAttendanceEngine(), services.NewAttendanceCache())
		auditSvc := services.NewAuditService(filepath.Join(t.TempDir(), "audit.json"), nil)
		userSvc := services.NewUserService(filepath.Join(t.TempDir(), "users.json"), nil)
		SetupRoutes(app, service, auditSvc, userSvc, services.NewRuleService(nil))
		return app
	}

	// BioTime hidup -> online (tanpa login, endpoint publik)
	app := newStatusApp(t, true)
	resp, err := app.Test(httptest.NewRequest("GET", "/api/biotime-status", nil), 15000)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("status gagal: %v code=%v", err, resp.StatusCode)
	}
	var okBody struct {
		Biotime string `json:"biotime"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &okBody); err != nil || okBody.Biotime != "online" {
		t.Fatalf("mau online, dapat: %s", body)
	}

	// BioTime mati -> offline tapi tetap HTTP 200
	appDown := newStatusApp(t, false)
	respDown, err := appDown.Test(httptest.NewRequest("GET", "/api/biotime-status", nil), 15000)
	if err != nil || respDown.StatusCode != 200 {
		t.Fatalf("status offline gagal: %v code=%v", err, respDown.StatusCode)
	}
	var downBody struct {
		Biotime string `json:"biotime"`
	}
	bodyDown, _ := io.ReadAll(respDown.Body)
	if err := json.Unmarshal(bodyDown, &downBody); err != nil || downBody.Biotime != "offline" {
		t.Fatalf("mau offline, dapat: %s", bodyDown)
	}
}
