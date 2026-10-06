package models

import "time"

// Definisi Konstanta Status Presensi & Apel ASN
const (
	StatusTepatWaktu  = "Hadir Apel Pagi"
	StatusTerlambat   = "Terlambat"
	StatusBelumScan   = "Belum Scan"
	StatusSahApelSore = "Hadir Apel Sore"
	StatusPulangCepat = "Pulang Cepat"
	StatusLewatCutoff = "Lewat Cutoff (Ditolak)"
)

// DailyAttendanceLog menyimpan hasil kalkulasi presensi per pegawai untuk 1 hari kerja
type DailyAttendanceLog struct {
	EmpCode         string               `json:"emp_code"`
	Name            string               `json:"name"`
	Department      string               `json:"department"`
	Date            string               `json:"date"`     // Format: YYYY-MM-DD
	DayName         string               `json:"day_name"` // Senin, Selasa, dll
	IsFriday        bool                 `json:"is_friday"`

	CheckInTime     *time.Time           `json:"check_in_time"`
	CheckInDisplay  string               `json:"check_in_display"`  // "06:58:21"
	CheckInStatus   string               `json:"check_in_status"`   // Tepat Waktu / Terlambat
	LateMinutes     int                  `json:"late_minutes"`      // Menit keterlambatan

	CheckOutTime    *time.Time           `json:"check_out_time"`
	CheckOutDisplay string               `json:"check_out_display"` // "15:35:10"
	CheckOutStatus  string               `json:"check_out_status"`  // Sah Hadir Apel Sore / Lewat Cutoff / Pulang Cepat

	IsAnomaly          bool                 `json:"is_anomaly"`
	AnomalyReason      string               `json:"anomaly_reason"`
	AuditStatus        string               `json:"audit_status"` // "Terverifikasi" / "Anomali Barisan"
	AuditPagiStatus    string               `json:"audit_pagi_status"`
	AuditSoreStatus    string               `json:"audit_sore_status"`
	IsApelPagiVerified bool                 `json:"is_apel_pagi_verified"`
	IsApelSoreVerified bool                 `json:"is_apel_sore_verified"`

	RawTransactions []BioTimeTransaction `json:"raw_transactions"`
}

// AttendanceSummary menyimpan metrik ringkasan agregat untuk tampilan dashboard
type AttendanceSummary struct {
	TotalPegawai     int                  `json:"total_pegawai"`
	TepatWaktuCount  int                  `json:"tepat_waktu_count"`
	TerlambatCount   int                  `json:"terlambat_count"`
	ApelSoreCount    int                  `json:"apel_sore_count"`
	AnomaliCount     int                  `json:"anomali_count"`
	KepatuhanPercent float64              `json:"kepatuhan_percent"`
	ErrorMessage     string               `json:"error_message,omitempty"` // Menyimpan pesan error jika gagal mengambil data dari BioTime

	Logs             []DailyAttendanceLog `json:"logs"`
}

// ChartSeries menyimpan 1 kelompok data seri untuk Apexcharts
type ChartSeries struct {
	Name string `json:"name"`
	Data []int  `json:"data"`
}

// ChartDistribution menyimpan payload data untuk grafik donut distribusi kehadiran
type ChartDistribution struct {
	TepatWaktu int      `json:"tepat_waktu"`
	Terlambat  int      `json:"terlambat"`
	ApelSore   int      `json:"apel_sore"`
	Anomali    int      `json:"anomali"`
	Series     []int    `json:"series"`
	Labels     []string `json:"labels"`
}

// ChartDataResponse menyimpan payload data untuk grafik tren kehadiran
type ChartDataResponse struct {
	Period       string             `json:"period"` // daily, weekly, monthly, annual
	PeriodLabel  string             `json:"period_label"`
	Categories   []string           `json:"categories"`
	Series       []ChartSeries      `json:"series"`
	Distribution *ChartDistribution `json:"distribution,omitempty"`
}
