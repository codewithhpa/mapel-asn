package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"dashboard-presensi/internal/config"

	"github.com/gofiber/fiber/v2"
)

// RenderDevice menampilkan halaman Kelola Device (status + aksi transfer BioTime).
func (h *DashboardHandler) RenderDevice(c *fiber.Ctx) error {
	ctx := context.Background()

	terminals, err := h.biotimeClient.FetchTerminals(ctx)
	if err != nil {
		log.Printf("[Device] Gagal menarik terminal: %v\n", err)
	}

	online, totalUser, totalFace := 0, 0, 0
	for _, t := range terminals {
		if t.IsOnline() {
			online++
		}
		totalUser += int(t.UserCount)
		totalFace += int(t.FaceCount)
	}

	// Pembanding sisi server: jumlah pegawai + yang punya template wajah (vl_face).
	// Kalau angka wajah device > server, berarti ada template yang numpang di device
	// tapi belum naik ke server (enroll langsung di device tanpa upload).
	serverUser, serverFace, serverOK := 0, 0, false
	if empResp, err := h.biotimeClient.FetchEmployees(ctx, 1, 500, "", 0); err == nil && empResp != nil {
		serverOK = true
		serverUser = empResp.Count
		for _, e := range empResp.Data {
			if e.VLFace != "" && e.VLFace != "-" {
				serverFace++
			}
		}
	} else {
		log.Printf("[Device] Gagal menarik pegawai pembanding: %v\n", err)
	}

	return c.Render("device", fiber.Map{
		"BioTimeVersion":  config.GetEnv("BIOTIME_VERSION", "8.5"),
		"Title":           "Kelola Device Face Recognition",
		"ActiveMenu":      "device",
		"CurrentUser":     c.Locals("currentUser"),
		"Terminals":       terminals,
		"OnlineCount":     online,
		"TotalUserCount":  totalUser,
		"TotalFaceCount":  totalFace,
		"ServerUserCount": serverUser,
		"ServerFaceCount": serverFace,
		"ServerStatsOK":   serverOK,
	}, "layouts/main")
}

// GetTerminalsJSON mengembalikan daftar device untuk refresh parsial.
func (h *DashboardHandler) GetTerminalsJSON(c *fiber.Ctx) error {
	ctx := context.Background()
	terminals, err := h.biotimeClient.FetchTerminals(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "success", "data": terminals})
}

// PostUploadTransaction memaksa device upload ulang tap tertahan ke BioTime.
func (h *DashboardHandler) PostUploadTransaction(c *fiber.Ctx) error {
	ctx := context.Background()

	var ids []int64
	if tid := c.Params("id"); tid != "" {
		var one int64
		if _, err := fmt.Sscanf(tid, "%d", &one); err != nil || one <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "ID device tidak valid"})
		}
		ids = []int64{one}
	} else {
		var req struct {
			TerminalIDs []int64 `json:"terminal_ids"`
		}
		if err := c.BodyParser(&req); err != nil || len(req.TerminalIDs) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Pilih minimal 1 device"})
		}
		if len(req.TerminalIDs) > 50 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Maksimal 50 device per perintah"})
		}
		ids = req.TerminalIDs
	}

	names := h.terminalNames(ctx, ids)
	if err := h.biotimeClient.UploadTransactions(ctx, ids); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": fmt.Sprintf("Gagal memicu upload: %v", err)})
	}

	_ = h.auditService.RecordMasterAction(
		"DEVICE_UPLOAD",
		h.getCurrentUserName(c),
		"", "",
		fmt.Sprintf("Tarik Transaksi manual dari device: %s", strings.Join(names, ", ")),
		c.IP(),
	)

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": fmt.Sprintf("Perintah upload terkirim ke %d device — tap tertahan masuk ±1 menit", len(ids)),
	})
}

// PostUploadUsers memaksa device upload data user + template biometrik ke server.
// Setara tombol BioTime: Data Transfer -> Upload User Data.
// Berbeda dengan Tarik Transaksi, efeknya TIDAK instan: device mengunggah saat
// polling berikutnya, dan BioTime bisa membuat record pegawai BARU bila PIN di
// device tidak sama dengan NIP/emp_code di server — wajib cek duplikat sesudahnya.
func (h *DashboardHandler) PostUploadUsers(c *fiber.Ctx) error {
	ctx := context.Background()

	var ids []int64
	if tid := c.Params("id"); tid != "" {
		var one int64
		if _, err := fmt.Sscanf(tid, "%d", &one); err != nil || one <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "ID device tidak valid"})
		}
		ids = []int64{one}
	} else {
		var req struct {
			TerminalIDs []int64 `json:"terminal_ids"`
		}
		if err := c.BodyParser(&req); err != nil || len(req.TerminalIDs) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Pilih minimal 1 device"})
		}
		if len(req.TerminalIDs) > 50 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Maksimal 50 device per perintah"})
		}
		ids = req.TerminalIDs
	}

	names := h.terminalNames(ctx, ids)
	if err := h.biotimeClient.UploadUsersFromDevice(ctx, ids); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": fmt.Sprintf("Gagal memicu upload user: %v", err)})
	}

	_ = h.auditService.RecordMasterAction(
		"DEVICE_UPLOAD_USERS",
		h.getCurrentUserName(c),
		"", "",
		fmt.Sprintf("Tarik Data User dari device ke server: %s", strings.Join(names, ", ")),
		c.IP(),
	)
	h.attendanceService.InvalidateEmployeeCache()

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": fmt.Sprintf("Perintah terkirim ke %d device — data user masuk ±1 menit. Cek Master Pegawai untuk duplikat bila PIN device tidak sama dengan NIP", len(ids)),
	})
}

// PostResyncEmployees menandai pegawai agar datanya di-sync ulang ke device.
func (h *DashboardHandler) PostResyncEmployees(c *fiber.Ctx) error {
	ctx := context.Background()

	var req struct {
		EmployeeIDs []int64 `json:"employee_ids"`
	}
	if err := c.BodyParser(&req); err != nil || len(req.EmployeeIDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Pilih minimal 1 pegawai"})
	}
	if len(req.EmployeeIDs) > 500 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Maksimal 500 pegawai per perintah"})
	}

	if err := h.biotimeClient.ResyncEmployeesToDevice(ctx, req.EmployeeIDs); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": fmt.Sprintf("Gagal sync ke device: %v", err)})
	}

	_ = h.auditService.RecordMasterAction(
		"DEVICE_RESYNC",
		h.getCurrentUserName(c),
		"", "",
		fmt.Sprintf("Sync %d pegawai ke device (resync_to_device)", len(req.EmployeeIDs)),
		c.IP(),
	)
	h.attendanceService.InvalidateEmployeeCache()

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": fmt.Sprintf("%d pegawai ditandai sync — device mengambil datanya saat polling", len(req.EmployeeIDs)),
	})
}

// GetBioTimeStatus mengecek konektivitas live ke BioTime untuk ribbon header.
// Selalu HTTP 200 (ini status probe, bukan health app): lapangan "biotime" = online/offline.
func (h *DashboardHandler) GetBioTimeStatus(c *fiber.Ctx) error {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	_, err := h.biotimeClient.FetchTerminals(ctx)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		log.Printf("[BioTimeStatus] offline (%d ms): %v\n", latency, err)
		return c.JSON(fiber.Map{
			"status":     "success",
			"biotime":    "offline",
			"latency_ms": latency,
			"message":    "BioTime tidak terjangkau",
		})
	}
	return c.JSON(fiber.Map{
		"status":     "success",
		"biotime":    "online",
		"latency_ms": latency,
	})
}

// terminalNames memetakan ID device ke nama tampilan untuk audit log.
func (h *DashboardHandler) terminalNames(ctx context.Context, ids []int64) []string {
	terminals, err := h.biotimeClient.FetchTerminals(ctx)
	if err != nil {
		names := make([]string, 0, len(ids))
		for _, id := range ids {
			names = append(names, fmt.Sprintf("#%d", id))
		}
		return names
	}
	byID := map[int64]string{}
	for _, t := range terminals {
		byID[t.ID] = fmt.Sprintf("%s (%s)", t.DisplayName(), t.SN)
	}
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		if n, ok := byID[id]; ok {
			names = append(names, n)
		} else {
			names = append(names, fmt.Sprintf("#%d", id))
		}
	}
	return names
}
