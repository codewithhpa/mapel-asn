package handlers

import (
	"net/url"

	"dashboard-presensi/internal/models"

	"github.com/gofiber/fiber/v2"
)

// Kunci filter status dashboard. Setiap pegawai masuk tepat satu kategori
// (berdasarkan status scan pagi), sehingga jumlah semua kategori = total pegawai.
const (
	statusFilterAll        = "all"
	statusFilterTepat      = "tepat"
	statusFilterTerlambat  = "terlambat"
	statusFilterIzin       = "izin"
	statusFilterTidakHadir = "tidak_hadir"
)

// statusTab adalah 1 segmen pada kontrol filter status di dashboard.
type statusTab struct {
	Key    string
	Label  string
	Count  int
	URL    string
	Active bool
}

// periodTab adalah 1 segmen pilihan periode cepat di dashboard.
type periodTab struct {
	Key    string
	Label  string
	URL    string
	Active bool
}

// normalizeStatusFilter memastikan nilai query ?status= selalu salah satu kunci valid.
func normalizeStatusFilter(s string) string {
	switch s {
	case statusFilterTepat, statusFilterTerlambat, statusFilterIzin, statusFilterTidakHadir:
		return s
	}
	return statusFilterAll
}

// statusFilterLabel memberi nama tampilan untuk kunci filter status (dipakai di judul cetak).
func statusFilterLabel(status string) string {
	switch status {
	case statusFilterTepat:
		return "Tepat Waktu"
	case statusFilterTerlambat:
		return "Terlambat"
	case statusFilterIzin:
		return "Izin / DL"
	case statusFilterTidakHadir:
		return "Tidak Hadir"
	}
	return ""
}

// attendanceStatusBucket memetakan 1 log presensi ke kategori filter ("" bila di luar kategori).
func attendanceStatusBucket(l models.DailyAttendanceLog) string {
	switch {
	case l.CheckInStatus == models.StatusTerlambat:
		return statusFilterTerlambat
	case l.CheckInStatus == models.StatusTepatWaktu || l.CheckInStatus == "Tepat Waktu":
		return statusFilterTepat
	case models.IsExcusedLeaveStatus(l.CheckInStatus):
		return statusFilterIzin
	case l.CheckInStatus == models.StatusBelumScan:
		return statusFilterTidakHadir
	}
	return ""
}

// filterLogsByStatus mengembalikan log yang masuk kategori status (all = semua).
func filterLogsByStatus(logs []models.DailyAttendanceLog, status string) []models.DailyAttendanceLog {
	if status == statusFilterAll {
		return logs
	}
	out := make([]models.DailyAttendanceLog, 0, len(logs))
	for _, l := range logs {
		if attendanceStatusBucket(l) == status {
			out = append(out, l)
		}
	}
	return out
}

// countLogsByStatus menghitung jumlah pegawai per kategori filter.
func countLogsByStatus(logs []models.DailyAttendanceLog) map[string]int {
	counts := map[string]int{}
	for _, l := range logs {
		if b := attendanceStatusBucket(l); b != "" {
			counts[b]++
		}
	}
	return counts
}

// buildStatusTabs menyusun segmen filter beserta URL yang tetap membawa parameter periode.
func buildStatusTabs(c *fiber.Ctx, logs []models.DailyAttendanceLog, active string) []statusTab {
	counts := countLogsByStatus(logs)
	defs := []struct{ key, label string }{
		{statusFilterAll, "Semua"},
		{statusFilterTepat, "Tepat Waktu"},
		{statusFilterTerlambat, "Terlambat"},
		{statusFilterIzin, "Izin / DL"},
		{statusFilterTidakHadir, "Tidak Hadir"},
	}
	tabs := make([]statusTab, 0, len(defs))
	for _, d := range defs {
		count := counts[d.key]
		if d.key == statusFilterAll {
			count = len(logs)
		}
		tabs = append(tabs, statusTab{
			Key:    d.key,
			Label:  d.label,
			Count:  count,
			URL:    buildStatusURL(c, d.key),
			Active: d.key == active,
		})
	}
	return tabs
}

// buildPeriodTabs menyusun segmen periode. Setiap link menghapus filter bulan/tanggal
// tapi mempertahankan filter status, sehingga kategori yang dipilih tidak hilang diam-diam.
func buildPeriodTabs(c *fiber.Ctx, active string) []periodTab {
	defs := []struct{ key, label string }{
		{"today", "Hari Ini"},
		{"this_week", "Minggu Ini"},
		{"this_month", "Bulan Ini"},
		{"all", "Tahun Ini"},
	}
	tabs := make([]periodTab, 0, len(defs))
	for _, d := range defs {
		tabs = append(tabs, periodTab{
			Key:    d.key,
			Label:  d.label,
			URL:    buildQueryURL(c, "/", map[string]string{"period": d.key, "month": "", "start_date": "", "end_date": "", "date": ""}),
			Active: d.key == active,
		})
	}
	return tabs
}

// buildStatusURL menyalin query aktif lalu mengganti/menghapus ?status=.
func buildStatusURL(c *fiber.Ctx, status string) string {
	if status == statusFilterAll {
		return buildQueryURL(c, "/", nil, "status")
	}
	return buildQueryURL(c, "/", map[string]string{"status": status})
}

// buildQueryURL menyalin query aktif, lalu menimpa key di set (nilai kosong = hapus) dan menghapus key di del.
// Parameter lain (periode, bulan, status) ikut terbawa.
func buildQueryURL(c *fiber.Ctx, path string, set map[string]string, del ...string) string {
	q := url.Values{}
	c.Context().QueryArgs().VisitAll(func(k, v []byte) {
		q.Set(string(k), string(v))
	})
	for k, v := range set {
		if v == "" {
			q.Del(k)
		} else {
			q.Set(k, v)
		}
	}
	for _, k := range del {
		q.Del(k)
	}
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}
