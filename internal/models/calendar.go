package models

import "time"

// Tipe event kalender kedinasan.
const (
	CalendarTypeLiburNasional = "libur_nasional" // SKB 3 Menteri + SE Bupati
	CalendarTypeCutiBersama   = "cuti_bersama"   // SKB 3 Menteri + SE Bupati
	CalendarTypeJamKhusus     = "jam_kerja_ramadhan" // Rentang jam kerja Ramadhan (override jam + tanggal)
)

// CalendarTypes daftar tipe valid untuk validasi input.
var CalendarTypes = []string{
	CalendarTypeLiburNasional,
	CalendarTypeCutiBersama,
	CalendarTypeJamKhusus,
}

// CalendarEventLabel nama tampilan per tipe.
func CalendarEventLabel(t string) string {
	switch t {
	case CalendarTypeLiburNasional:
		return "Hari Libur Nasional"
	case CalendarTypeCutiBersama:
		return "Cuti Bersama"
	case CalendarTypeJamKhusus:
		return "Jam Kerja Ramadhan"
	}
	return t
}

// IsCalendarHolidayType true bila tipe event membuat tanggal menjadi hari libur.
func IsCalendarHolidayType(t string) bool {
	return t == CalendarTypeLiburNasional || t == CalendarTypeCutiBersama
}

// CalendarEvent 1 entri kalender kedinasan (bisa rentang tanggal, mis. Idulfitri 21-22 Maret).
// Kolom jam hanya dipakai tipe jam_kerja_ramadhan; string kosong = pakai aturan jam kerja default.
type CalendarEvent struct {
	ID                   int64     `json:"id"`
	Type                 string    `json:"type"`
	Name                 string    `json:"name"`
	StartDate            string    `json:"start_date"` // YYYY-MM-DD
	EndDate              string    `json:"end_date"`   // YYYY-MM-DD (>= start)
	WorkStartTime        string    `json:"work_start_time,omitempty"`
	CheckinToleranceTime string    `json:"checkin_tolerance_time,omitempty"`
	CheckoutTimeMonThu   string    `json:"checkout_time_mon_thu,omitempty"`
	CheckoutCutoffMonThu string    `json:"checkout_cutoff_mon_thu,omitempty"`
	CheckoutTimeFri      string    `json:"checkout_time_fri,omitempty"`
	CheckoutCutoffFri    string    `json:"checkout_cutoff_fri,omitempty"`
	CreatedBy            string    `json:"created_by,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// CalendarEventRequest payload create/update event kalender.
type CalendarEventRequest struct {
	Type                 string `json:"type"`
	Name                 string `json:"name"`
	StartDate            string `json:"start_date"`
	EndDate              string `json:"end_date"`
	WorkStartTime        string `json:"work_start_time,omitempty"`
	CheckinToleranceTime string `json:"checkin_tolerance_time,omitempty"`
	CheckoutTimeMonThu   string `json:"checkout_time_mon_thu,omitempty"`
	CheckoutCutoffMonThu string `json:"checkout_cutoff_mon_thu,omitempty"`
	CheckoutTimeFri      string `json:"checkout_time_fri,omitempty"`
	CheckoutCutoffFri    string `json:"checkout_cutoff_fri,omitempty"`
}
