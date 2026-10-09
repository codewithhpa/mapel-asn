package handlers

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"dashboard-presensi/internal/models"

	"github.com/gofiber/fiber/v2"
)

func statusTestLogs() []models.DailyAttendanceLog {
	return []models.DailyAttendanceLog{
		{EmpCode: "1", CheckInStatus: "Hadir Apel Pagi"},
		{EmpCode: "2", CheckInStatus: "Tepat Waktu"},
		{EmpCode: "3", CheckInStatus: "Terlambat"},
		{EmpCode: "4", CheckInStatus: "Belum Scan"},
		{EmpCode: "5", CheckInStatus: "Dinas Luar (DL)"},
		{EmpCode: "6", CheckInStatus: "Belum Scan", CheckOutStatus: "Pulang Cepat"},
	}
}

func TestNormalizeStatusFilter(t *testing.T) {
	cases := map[string]string{
		"":            statusFilterAll,
		"all":         statusFilterAll,
		"tepat":       statusFilterTepat,
		"terlambat":   statusFilterTerlambat,
		"izin":        statusFilterIzin,
		"tidak_hadir": statusFilterTidakHadir,
		"<script>":    statusFilterAll,
		"Hadir Apel":  statusFilterAll,
	}
	for in, want := range cases {
		if got := normalizeStatusFilter(in); got != want {
			t.Errorf("normalizeStatusFilter(%q) = %q, mau %q", in, got, want)
		}
	}
}

func TestStatusBucketsAreExclusiveAndCoverAll(t *testing.T) {
	logs := statusTestLogs()
	counts := countLogsByStatus(logs)

	want := map[string]int{
		statusFilterTepat:      2,
		statusFilterTerlambat:  1,
		statusFilterIzin:       1,
		statusFilterTidakHadir: 2,
	}
	sum := 0
	for k, n := range want {
		if counts[k] != n {
			t.Errorf("kategori %s: mau %d, dapat %d", k, n, counts[k])
		}
		sum += counts[k]
	}
	if sum != len(logs) {
		t.Errorf("total kategori %d harus sama dengan jumlah pegawai %d", sum, len(logs))
	}
}

func TestFilterLogsByStatus(t *testing.T) {
	logs := statusTestLogs()

	if got := filterLogsByStatus(logs, statusFilterAll); len(got) != len(logs) {
		t.Errorf("semua harus %d baris, dapat %d", len(logs), len(got))
	}
	got := filterLogsByStatus(logs, statusFilterTidakHadir)
	if len(got) != 2 {
		t.Fatalf("tidak_hadir harus 2 baris, dapat %d", len(got))
	}
	for _, l := range got {
		if l.CheckInStatus != "Belum Scan" {
			t.Errorf("baris salah masuk tidak_hadir: %+v", l)
		}
	}
}

func TestBuildStatusURL_KeepsPeriodParams(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString(buildStatusURL(c, statusFilterTerlambat) + "|" + buildStatusURL(c, statusFilterAll))
	})

	req := httptest.NewRequest("GET", "/?month=2026-10&status=tepat", nil)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("request gagal: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	want := "/?month=2026-10&status=terlambat|/?month=2026-10"
	if string(body) != want {
		t.Errorf("url = %q, mau %q", string(body), want)
	}
}

func TestPeriodTabs_ClearMonthKeepStatus(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		out := ""
		for _, p := range buildPeriodTabs(c, "today") {
			out += p.Key + "=" + p.URL + "\n"
		}
		return c.SendString(out)
	})

	req := httptest.NewRequest("GET", "/?month=2026-08&status=terlambat", nil)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("request gagal: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	got := string(body)
	if strings.Contains(got, "month=2026") {
		t.Errorf("tab periode harus menghapus month: %s", got)
	}
	if !strings.Contains(got, "status=terlambat") {
		t.Errorf("tab periode harus mempertahankan status: %s", got)
	}
	if !strings.Contains(got, "all=/?period=all&status=terlambat") {
		t.Errorf("tab Tahun Ini salah: %s", got)
	}
}

func TestPrintURL_MirrorsScreenQuery(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString(buildQueryURL(c, "/presensi/cetak", nil))
	})
	req := httptest.NewRequest("GET", "/?period=this_week&status=tepat", nil)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("request gagal: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if want := "/presensi/cetak?period=this_week&status=tepat"; string(body) != want {
		t.Errorf("print url = %q, mau %q", string(body), want)
	}
}
