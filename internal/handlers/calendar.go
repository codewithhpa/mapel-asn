package handlers

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"

	"dashboard-presensi/internal/models"
	"dashboard-presensi/internal/services"

	"github.com/gofiber/fiber/v2"
)

// CalendarTypeBadgeFunc render badge HTML per tipe event (dipakai template aturan-presensi).
func CalendarTypeBadgeFunc(t string) template.HTML {
	switch t {
	case models.CalendarTypeLiburNasional:
		return template.HTML(`<span class="badge bg-danger text-white"><i class="bi bi-x-circle-fill me-1"></i>Libur Nasional</span>`)
	case models.CalendarTypeCutiBersama:
		return template.HTML(`<span class="badge bg-warning text-dark"><i class="bi bi-moon-stars-fill me-1"></i>Cuti Bersama</span>`)
	case models.CalendarTypeJamKhusus:
		return template.HTML(`<span class="badge bg-info text-dark"><i class="bi bi-clock-fill me-1"></i>Jam Kerja Ramadhan</span>`)
	}
	return template.HTML(template.HTMLEscapeString(t))
}

// shortMonthID singkatan bulan Indonesia.
func shortMonthID(m time.Month) string {
	names := []string{"", "Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
	if m < 1 || m > 12 {
		return ""
	}
	return names[m]
}

// ShortDateIDFunc format tanggal "2026-03-21" -> "21 Mar 2026".
func ShortDateIDFunc(dateStr string) string {
	t, err := time.Parse(time.DateOnly, strings.TrimSpace(dateStr))
	if err != nil {
		return dateStr
	}
	return fmt.Sprintf("%d %s %d", t.Day(), shortMonthID(t.Month()), t.Year())
}

// ShortRangeIDFunc format rentang: sama -> "21 Mar 2026"; sebulan -> "21-22 Mar 2026"; beda -> "30 Des 2026 - 2 Jan 2027".
func ShortRangeIDFunc(startStr, endStr string) string {
	startStr, endStr = strings.TrimSpace(startStr), strings.TrimSpace(endStr)
	if startStr == "" {
		return endStr
	}
	if endStr == "" || endStr == startStr {
		return ShortDateIDFunc(startStr)
	}
	s, err1 := time.Parse(time.DateOnly, startStr)
	e, err2 := time.Parse(time.DateOnly, endStr)
	if err1 != nil || err2 != nil {
		return startStr + " s/d " + endStr
	}
	if s.Year() == e.Year() && s.Month() == e.Month() {
		return fmt.Sprintf("%d-%d %s %d", s.Day(), e.Day(), shortMonthID(s.Month()), s.Year())
	}
	return ShortDateIDFunc(startStr) + " - " + ShortDateIDFunc(endStr)
}

// CalendarHoursChipsFunc ringkasan jam override sebagai chip kompak (kosong bila bukan jam_kerja_ramadhan).
func CalendarHoursChipsFunc(ev models.CalendarEvent) template.HTML {
	if ev.Type != models.CalendarTypeJamKhusus {
		return template.HTML(`<span class="text-muted">—</span>`)
	}
	var b strings.Builder
	chip := func(label, v string) {
		if v == "" {
			return
		}
		b.WriteString(`<span class="cal-hour-chip">` + template.HTMLEscapeString(label+" "+v) + `</span>`)
	}
	chip("masuk", ev.WorkStartTime)
	chip("tol.", ev.CheckinToleranceTime)
	chip("Sn-Km", ev.CheckoutTimeMonThu)
	chip("cutoff", ev.CheckoutCutoffMonThu)
	chip("Jum", ev.CheckoutTimeFri)
	chip("cutoff Jum", ev.CheckoutCutoffFri)
	if b.Len() == 0 {
		return template.HTML(`<span class="text-muted small">aturan default</span>`)
	}
	return template.HTML(b.String())
}

// CalendarHoursLabelFunc ringkasan jam override (kosong bila tanpa override).
func CalendarHoursLabelFunc(ev models.CalendarEvent) string {
	if ev.Type != models.CalendarTypeJamKhusus {
		return "—"
	}
	var parts []string
	if ev.WorkStartTime != "" {
		parts = append(parts, "masuk "+ev.WorkStartTime)
	}
	if ev.CheckinToleranceTime != "" {
		parts = append(parts, "tol. "+ev.CheckinToleranceTime)
	}
	if ev.CheckoutTimeMonThu != "" {
		parts = append(parts, "pulang Sn-Km "+ev.CheckoutTimeMonThu)
	}
	if ev.CheckoutTimeFri != "" {
		parts = append(parts, "Jum "+ev.CheckoutTimeFri)
	}
	if len(parts) == 0 {
		return "aturan default"
	}
	return strings.Join(parts, " · ")
}

func (h *DashboardHandler) calendar() *services.CalendarService {
	return h.attendanceService.CalendarService()
}

// todayCalendarInfo menyusun info kalender hari ini untuk banner dashboard/TV.
func (h *DashboardHandler) todayCalendarInfo() (holiday, specialHours string) {
	cal := h.attendanceService.CalendarService()
	if cal == nil {
		return "", ""
	}
	today := time.Now()
	for _, ev := range cal.GetEventsForDate(today) {
		switch {
		case models.IsCalendarHolidayType(ev.Type):
			if holiday == "" {
				holiday = models.CalendarEventLabel(ev.Type) + ": " + ev.Name
			}
		case ev.Type == models.CalendarTypeJamKhusus:
			if specialHours == "" {
				h := ev.WorkStartTime
				if h == "" {
					h = "aturan default"
				}
				specialHours = fmt.Sprintf("%s — masuk %s", ev.Name, h)
			}
		}
	}
	return holiday, specialHours
}

// GetCalendarJSON mengembalikan daftar event kalender per tahun (admin).
func (h *DashboardHandler) GetCalendarJSON(c *fiber.Ctx) error {
	cal := h.calendar()
	if cal == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "error", "message": "Kalender kedinasan tidak aktif"})
	}
	year, err := strconv.Atoi(c.Query("year"))
	if err != nil || year < 2000 || year > 2200 {
		year = time.Now().Year()
	}
	events, err := cal.ListEvents(year)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "success", "year": year, "data": events})
}

// PostCalendarEvent menambah event kalender kedinasan (admin).
func (h *DashboardHandler) PostCalendarEvent(c *fiber.Ctx) error {
	cal := h.calendar()
	if cal == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "error", "message": "Kalender kedinasan tidak aktif"})
	}
	var req models.CalendarEventRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Format payload tidak valid"})
	}
	created, err := cal.CreateEvent(req, h.getCurrentUserName(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}
	_ = h.auditService.RecordMasterAction(
		"KALENDER_KEDINASAN",
		h.getCurrentUserName(c),
		"", "",
		fmt.Sprintf("Tambah event kalender: [%s] %s (%s s/d %s)", created.Type, created.Name, created.StartDate, created.EndDate),
		c.IP(),
	)
	h.attendanceService.InvalidateEmployeeCache()
	// Kalender berubah: ringkasan ter-cache dihitung dengan aturan lama, buang.
	h.attendanceService.InvalidateCache()
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"status": "success", "message": "Event kalender berhasil ditambahkan", "data": created})
}

// PutCalendarEvent memperbarui event kalender kedinasan (admin).
func (h *DashboardHandler) PutCalendarEvent(c *fiber.Ctx) error {
	cal := h.calendar()
	if cal == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "error", "message": "Kalender kedinasan tidak aktif"})
	}
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "ID event tidak valid"})
	}
	var req models.CalendarEventRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "Format payload tidak valid"})
	}
	updated, err := cal.UpdateEvent(id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}
	_ = h.auditService.RecordMasterAction(
		"KALENDER_KEDINASAN",
		h.getCurrentUserName(c),
		"", "",
		fmt.Sprintf("Ubah event kalender #%d: [%s] %s (%s s/d %s)", updated.ID, updated.Type, updated.Name, updated.StartDate, updated.EndDate),
		c.IP(),
	)
	h.attendanceService.InvalidateEmployeeCache()
	// Kalender berubah: ringkasan ter-cache dihitung dengan aturan lama, buang.
	h.attendanceService.InvalidateCache()
	return c.JSON(fiber.Map{"status": "success", "message": "Event kalender berhasil diperbarui", "data": updated})
}

// DeleteCalendarEvent menghapus event kalender kedinasan (admin).
func (h *DashboardHandler) DeleteCalendarEvent(c *fiber.Ctx) error {
	cal := h.calendar()
	if cal == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "error", "message": "Kalender kedinasan tidak aktif"})
	}
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": "ID event tidak valid"})
	}
	if err := cal.DeleteEvent(id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}
	_ = h.auditService.RecordMasterAction(
		"KALENDER_KEDINASAN",
		h.getCurrentUserName(c),
		"", "",
		fmt.Sprintf("Hapus event kalender #%d", id),
		c.IP(),
	)
	h.attendanceService.InvalidateEmployeeCache()
	// Kalender berubah: ringkasan ter-cache dihitung dengan aturan lama, buang.
	h.attendanceService.InvalidateCache()
	return c.JSON(fiber.Map{"status": "success", "message": "Event kalender berhasil dihapus"})
}
