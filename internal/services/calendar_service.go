package services

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"dashboard-presensi/internal/models"
)

// CalendarService mengelola kalender kedinasan (libur nasional, cuti bersama,
// jam khusus) dari MariaDB dengan fallback in-memory.
type CalendarService struct {
	mu       sync.RWMutex
	db       *sql.DB
	fallback []models.CalendarEvent
}

func NewCalendarService(db *sql.DB) *CalendarService {
	return &CalendarService{db: db}
}

func isValidCalendarType(t string) bool {
	switch t {
	case models.CalendarTypeLiburNasional, models.CalendarTypeCutiBersama,
		models.CalendarTypeJamKhusus:
		return true
	}
	return false
}

func validateEventDates(req models.CalendarEventRequest) (start, end time.Time, err error) {
	start, err = time.Parse(time.DateOnly, strings.TrimSpace(req.StartDate))
	if err != nil {
		return start, end, fmt.Errorf("tanggal mulai harus format YYYY-MM-DD")
	}
	if strings.TrimSpace(req.EndDate) == "" {
		req.EndDate = req.StartDate
	}
	end, err = time.Parse(time.DateOnly, strings.TrimSpace(req.EndDate))
	if err != nil {
		return start, end, fmt.Errorf("tanggal selesai harus format YYYY-MM-DD")
	}
	if end.Before(start) {
		return start, end, fmt.Errorf("tanggal selesai tidak boleh sebelum tanggal mulai")
	}
	return start, end, nil
}

func validateEventTimes(req models.CalendarEventRequest) error {
	if req.Type != models.CalendarTypeJamKhusus {
		return nil
	}
	for label, v := range map[string]string{
		"jam masuk":       req.WorkStartTime,
		"toleransi masuk": req.CheckinToleranceTime,
		"pulang Sen-Kam":  req.CheckoutTimeMonThu,
		"cutoff Sen-Kam":  req.CheckoutCutoffMonThu,
		"pulang Jumat":    req.CheckoutTimeFri,
		"cutoff Jumat":    req.CheckoutCutoffFri,
	} {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, err := time.Parse("15:04:05", v); err != nil {
			return fmt.Errorf("%s harus format HH:MM:SS (contoh 07:30:00)", label)
		}
	}
	return nil
}

// CreateEvent menambah event kalender baru.
func (s *CalendarService) CreateEvent(req models.CalendarEventRequest, createdBy string) (*models.CalendarEvent, error) {
	req.Type = strings.TrimSpace(strings.ToLower(req.Type))
	if !isValidCalendarType(req.Type) {
		return nil, fmt.Errorf("tipe harus salah satu: %s", strings.Join(models.CalendarTypes, ", "))
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return nil, errors.New("keterangan / nama event wajib diisi")
	}
	start, end, err := validateEventDates(req)
	if err != nil {
		return nil, err
	}
	if err := validateEventTimes(req); err != nil {
		return nil, err
	}

	ev := &models.CalendarEvent{
		Type:                 req.Type,
		Name:                 req.Name,
		StartDate:            start.Format(time.DateOnly),
		EndDate:              end.Format(time.DateOnly),
		WorkStartTime:        strings.TrimSpace(req.WorkStartTime),
		CheckinToleranceTime: strings.TrimSpace(req.CheckinToleranceTime),
		CheckoutTimeMonThu:   strings.TrimSpace(req.CheckoutTimeMonThu),
		CheckoutCutoffMonThu: strings.TrimSpace(req.CheckoutCutoffMonThu),
		CheckoutTimeFri:      strings.TrimSpace(req.CheckoutTimeFri),
		CheckoutCutoffFri:    strings.TrimSpace(req.CheckoutCutoffFri),
		CreatedBy:            createdBy,
	}

	if s.db != nil {
		res, err := s.db.Exec(`INSERT INTO calendar_events
			(type, name, start_date, end_date, work_start_time, checkin_tolerance_time,
			 checkout_time_mon_thu, checkout_cutoff_mon_thu, checkout_time_fri, checkout_cutoff_fri, created_by)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			ev.Type, ev.Name, ev.StartDate, ev.EndDate, ev.WorkStartTime, ev.CheckinToleranceTime,
			ev.CheckoutTimeMonThu, ev.CheckoutCutoffMonThu, ev.CheckoutTimeFri, ev.CheckoutCutoffFri, ev.CreatedBy)
		if err != nil {
			return nil, fmt.Errorf("gagal menyimpan event kalender: %w", err)
		}
		id, _ := res.LastInsertId()
		ev.ID = id
		return ev, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	ev.ID = int64(len(s.fallback) + 1)
	s.fallback = append(s.fallback, *ev)
	return ev, nil
}

// UpdateEvent memperbarui event kalender berdasarkan ID.
func (s *CalendarService) UpdateEvent(id int64, req models.CalendarEventRequest) (*models.CalendarEvent, error) {
	req.Type = strings.TrimSpace(strings.ToLower(req.Type))
	if !isValidCalendarType(req.Type) {
		return nil, fmt.Errorf("tipe harus salah satu: %s", strings.Join(models.CalendarTypes, ", "))
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return nil, errors.New("keterangan / nama event wajib diisi")
	}
	start, end, err := validateEventDates(req)
	if err != nil {
		return nil, err
	}
	if err := validateEventTimes(req); err != nil {
		return nil, err
	}

	if s.db != nil {
		// Cek keberadaan via SELECT dulu: UPDATE tanpa perubahan nilai di MariaDB
		// mengembalikan 0 rows affected, yang bukan berarti barisnya tidak ada.
		var exists int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM calendar_events WHERE id = ?`, id).Scan(&exists); err != nil {
			return nil, fmt.Errorf("gagal memeriksa event kalender: %w", err)
		}
		if exists == 0 {
			return nil, errors.New("event kalender tidak ditemukan")
		}
		_, err := s.db.Exec(`UPDATE calendar_events SET
			type = ?, name = ?, start_date = ?, end_date = ?,
			work_start_time = ?, checkin_tolerance_time = ?,
			checkout_time_mon_thu = ?, checkout_cutoff_mon_thu = ?,
			checkout_time_fri = ?, checkout_cutoff_fri = ?
			WHERE id = ?`,
			req.Type, req.Name, start.Format(time.DateOnly), end.Format(time.DateOnly),
			strings.TrimSpace(req.WorkStartTime), strings.TrimSpace(req.CheckinToleranceTime),
			strings.TrimSpace(req.CheckoutTimeMonThu), strings.TrimSpace(req.CheckoutCutoffMonThu),
			strings.TrimSpace(req.CheckoutTimeFri), strings.TrimSpace(req.CheckoutCutoffFri), id)
		if err != nil {
			return nil, fmt.Errorf("gagal memperbarui event kalender: %w", err)
		}
		ev, err := s.GetEventByID(id)
		if err != nil {
			return nil, err
		}
		return ev, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.fallback {
		if s.fallback[i].ID == id {
			s.fallback[i].Type = req.Type
			s.fallback[i].Name = req.Name
			s.fallback[i].StartDate = start.Format(time.DateOnly)
			s.fallback[i].EndDate = end.Format(time.DateOnly)
			s.fallback[i].WorkStartTime = strings.TrimSpace(req.WorkStartTime)
			s.fallback[i].CheckinToleranceTime = strings.TrimSpace(req.CheckinToleranceTime)
			s.fallback[i].CheckoutTimeMonThu = strings.TrimSpace(req.CheckoutTimeMonThu)
			s.fallback[i].CheckoutCutoffMonThu = strings.TrimSpace(req.CheckoutCutoffMonThu)
			s.fallback[i].CheckoutTimeFri = strings.TrimSpace(req.CheckoutTimeFri)
			s.fallback[i].CheckoutCutoffFri = strings.TrimSpace(req.CheckoutCutoffFri)
			ev := s.fallback[i]
			return &ev, nil
		}
	}
	return nil, errors.New("event kalender tidak ditemukan")
}

// DeleteEvent menghapus event kalender berdasarkan ID.
func (s *CalendarService) DeleteEvent(id int64) error {
	if s.db != nil {
		res, err := s.db.Exec(`DELETE FROM calendar_events WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("gagal menghapus event kalender: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errors.New("event kalender tidak ditemukan")
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.fallback {
		if s.fallback[i].ID == id {
			s.fallback = append(s.fallback[:i], s.fallback[i+1:]...)
			return nil
		}
	}
	return errors.New("event kalender tidak ditemukan")
}

func scanCalendarEvent(row interface{ Scan(...any) error }) (*models.CalendarEvent, error) {
	var ev models.CalendarEvent
	// Kolom DATE dari MariaDB datang sebagai time.Time (parseTime=True di DSN).
	// Scan ke string mentah menghasilkan "2026-01-01T00:00:00+07:00" (RFC3339) —
	// jadi scan ke time.Time lalu format ke YYYY-MM-DD yang dipakai engine & UI.
	var start, end time.Time
	err := row.Scan(&ev.ID, &ev.Type, &ev.Name, &start, &end,
		&ev.WorkStartTime, &ev.CheckinToleranceTime, &ev.CheckoutTimeMonThu, &ev.CheckoutCutoffMonThu,
		&ev.CheckoutTimeFri, &ev.CheckoutCutoffFri, &ev.CreatedBy, &ev.CreatedAt, &ev.UpdatedAt)
	if err != nil {
		return nil, err
	}
	ev.StartDate = start.Format(time.DateOnly)
	ev.EndDate = end.Format(time.DateOnly)
	return &ev, nil
}

const calendarCols = `id, type, name, start_date, end_date, work_start_time, checkin_tolerance_time,
	checkout_time_mon_thu, checkout_cutoff_mon_thu, checkout_time_fri, checkout_cutoff_fri, created_by, created_at, updated_at`

// GetEventByID mengambil 1 event berdasarkan ID.
func (s *CalendarService) GetEventByID(id int64) (*models.CalendarEvent, error) {
	if s.db != nil {
		return scanCalendarEvent(s.db.QueryRow(`SELECT `+calendarCols+` FROM calendar_events WHERE id = ?`, id))
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ev := range s.fallback {
		if ev.ID == id {
			e := ev
			return &e, nil
		}
	}
	return nil, errors.New("event kalender tidak ditemukan")
}

// ListEvents mengembalikan event yang menyentuh tahun berjalan (lintas tahun tetap masuk).
func (s *CalendarService) ListEvents(year int) ([]models.CalendarEvent, error) {
	if s.db != nil {
		rows, err := s.db.Query(`SELECT `+calendarCols+` FROM calendar_events
			WHERE YEAR(start_date) <= ? AND YEAR(end_date) >= ?
			ORDER BY start_date ASC, id ASC`, year, year)
		if err != nil {
			return nil, fmt.Errorf("gagal membaca kalender: %w", err)
		}
		defer rows.Close()
		var out []models.CalendarEvent
		for rows.Next() {
			ev, err := scanCalendarEvent(rows)
			if err != nil {
				return nil, err
			}
			out = append(out, *ev)
		}
		return out, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []models.CalendarEvent
	for _, ev := range s.fallback {
		if strings.HasPrefix(ev.StartDate, fmt.Sprintf("%d", year)) || strings.HasPrefix(ev.EndDate, fmt.Sprintf("%d", year)) ||
			(ev.StartDate <= fmt.Sprintf("%d-12-31", year) && ev.EndDate >= fmt.Sprintf("%d-01-01", year)) {
			out = append(out, ev)
		}
	}
	return out, nil
}

// GetEventsForDate mengembalikan semua event aktif pada tanggal tsb.
func (s *CalendarService) GetEventsForDate(date time.Time) []models.CalendarEvent {
	dStr := date.Format(time.DateOnly)
	var out []models.CalendarEvent
	if s.db != nil {
		rows, err := s.db.Query(`SELECT `+calendarCols+` FROM calendar_events
			WHERE start_date <= ? AND end_date >= ?
			ORDER BY FIELD(type, 'libur_nasional', 'cuti_bersama', 'jam_kerja_ramadhan'), id ASC`, dStr, dStr)
		if err != nil {
			return nil
		}
		defer rows.Close()
		for rows.Next() {
			ev, err := scanCalendarEvent(rows)
			if err != nil {
				continue
			}
			out = append(out, *ev)
		}
		return out
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ev := range s.fallback {
		if ev.StartDate <= dStr && ev.EndDate >= dStr {
			out = append(out, ev)
		}
	}
	return out
}

// IsHolidayDate true bila tanggal = libur nasional / cuti bersama (bukan akhir pekan — itu urusan engine).
func (s *CalendarService) IsHolidayDate(date time.Time) bool {
	for _, ev := range s.GetEventsForDate(date) {
		if models.IsCalendarHolidayType(ev.Type) {
			return true
		}
	}
	return false
}

// HolidayOnDate mengembalikan event libur/cuti pada tanggal tsb (nil bila bukan hari libur).
func (s *CalendarService) HolidayOnDate(date time.Time) *models.CalendarEvent {
	for _, ev := range s.GetEventsForDate(date) {
		if models.IsCalendarHolidayType(ev.Type) {
			e := ev
			return &e
		}
	}
	return nil
}

// SpecialHoursOnDate mengembalikan event jam_kerja_ramadhan pada tanggal tsb (nil bila tidak ada).
func (s *CalendarService) SpecialHoursOnDate(date time.Time) *models.CalendarEvent {
	for _, ev := range s.GetEventsForDate(date) {
		if ev.Type == models.CalendarTypeJamKhusus {
			e := ev
			return &e
		}
	}
	return nil
}

// HolidayDatesInRange mengembalikan map tanggal (YYYY-MM-DD) -> nama event untuk semua
// hari libur/cuti bersama dalam rentang. Dipakai engine & alpha agar hari libur tidak
// dihitung sebagai Alpha massal.
func (s *CalendarService) HolidayDatesInRange(start, end time.Time) map[string]models.CalendarEvent {
	out := map[string]models.CalendarEvent{}
	if end.Before(start) {
		return out
	}
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if ev := s.HolidayOnDate(d); ev != nil {
			out[d.Format(time.DateOnly)] = *ev
		}
	}
	return out
}
