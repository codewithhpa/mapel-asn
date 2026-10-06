package models

import "time"

// Status Verifikasi & Jenis Dispensasi Resmi
const (
	AuditStatusBelumDiverifikasi = "Belum Diverifikasi"
	AuditStatusHadirBarisan      = "Hadir di Barisan"
	AuditStatusAnomaliTapRun     = "Anomali Tap & Run"
	AuditStatusDinasLuar         = "Dinas Luar"
	AuditStatusSakit             = "Izin / Sakit"
	AuditStatusCuti              = "Cuti"

	// Kategori Sesi / Jenis Pengecualian Dispensasi
	DispensasiApelPagi  = "Apel Pagi"
	DispensasiApelSore  = "Apel Sore"
	DispensasiDinasLuar = "Dinas Luar"
	DispensasiCuti      = "Cuti"
	DispensasiSakit     = "Izin Sakit"

	// Keterangan lainnya (penjelasan wajib di kolom catatan); diakui sah seperti dispensasi resmi.
	DispensasiLainnya = "Lainnya"

	StatusDisahkanHadir = "Disahkan Hadir"
)

// EkinerjaLeaveType memetakan kode dispensasi ekinerja ke label tampil.
type EkinerjaLeaveType struct {
	Code  string
	Label string
}

// EkinerjaLeaveTypes adalah daftar jenis Tugas & Izin Resmi sesuai ekinerja.
// Nilai option di form dispensasi: "CODE|CODE - Label" (session=CODE agar kunci audit stabil).
var EkinerjaLeaveTypes = []EkinerjaLeaveType{
	{Code: "CS", Label: "CS - Cuti Sakit"},
	{Code: "DL", Label: "DL - Dinas Luar Daerah"},
	{Code: "CM", Label: "CM - Cuti Bersalin"},
	{Code: "CB", Label: "CB - Cuti Besar"},
	{Code: "CAP", Label: "CAP - Cuti karena Alasan Penting"},
	{Code: "CH", Label: "CH - Cuti Tahunan"},
	{Code: "CLTN", Label: "CLTN - Cuti Diluar Tanggungan Negara"},
	{Code: "CBA", Label: "CBA - Cuti Bersama"},
	{Code: "TUBEL", Label: "TUBEL - Tugas Belajar"},
	{Code: "DD", Label: "DD - Dinas Dalam Daerah"},
}

// ExcusedSessions mengembalikan seluruh sesi dispensasi yang diakui sah
// (3 legacy + 10 kode ekinerja + Lainnya). Legacy dipertahankan agar catatan lama tetap terbaca.
func ExcusedSessions() []string {
	sessions := []string{DispensasiDinasLuar, DispensasiCuti, DispensasiSakit}
	for _, t := range EkinerjaLeaveTypes {
		sessions = append(sessions, t.Code)
	}
	return append(sessions, DispensasiLainnya)
}

// IsExcusedLeaveStatus true bila status presensi adalah label cuti/dinas yang sah
// ("Dinas Luar (DL)", "Cuti", "Izin Sakit", label ekinerja "CODE - ...", atau "Lainnya").
func IsExcusedLeaveStatus(status string) bool {
	switch status {
	case "Dinas Luar (DL)", "Cuti", "Izin Sakit", DispensasiLainnya:
		return true
	}
	for _, t := range EkinerjaLeaveTypes {
		if status == t.Label {
			return true
		}
	}
	return false
}

// AuditRecord merepresentasikan catatan log audit fisik barisan sesuai standar ASN (AGENT.md 7.3)
type AuditRecord struct {
	ID             string    `json:"id"`
	Date           string    `json:"date"` // YYYY-MM-DD
	EmpCode        string    `json:"emp_code"`
	Name           string    `json:"name"`
	Department     string    `json:"department"`
	Session        string    `json:"session"` // "Apel Pagi", "Apel Sore", "Dinas Luar", "Cuti", "Izin Sakit", atau kode ekinerja (CS/DL/CM/CB/CAP/CH/CLTN/CBA/TUBEL/DD)
	PhysicalStatus string    `json:"physical_status"`
	Notes          string    `json:"notes"`
	VerifierName   string    `json:"verifier_name"`
	VerifierIP     string    `json:"verifier_ip"`
	CreatedAt      time.Time `json:"created_at"`
}

// AuditVerifyRequest merepresentasikan payload POST dari form dispensasi petugas
type AuditVerifyRequest struct {
	EmpCode        string `json:"emp_code"`
	Name           string `json:"name"`
	Department     string `json:"department"`
	Date           string `json:"date"`            // YYYY-MM-DD
	Session        string `json:"session"`         // "Apel Pagi", "Apel Sore", "Dinas Luar", "Cuti", "Izin Sakit"
	PhysicalStatus string `json:"physical_status"` // "Disahkan Hadir", "Dinas Luar (DL)", dll
	Notes          string `json:"notes"`
	VerifierName   string `json:"verifier_name"`
}
