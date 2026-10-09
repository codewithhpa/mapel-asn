package services

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"dashboard-presensi/internal/models"

	"github.com/xuri/excelize/v2"
)

// Batas baris per file import agar tidak menghantam BioTime sekaligus.
const EmployeeImportMaxRows = 1000

// Header kolom template (urutan tetap, dipakai saat generate + alias saat parse).
var EmployeeImportHeaders = []string{
	"NIP / ID Pegawai *",
	"Nama Depan *",
	"Nama Belakang",
	"Kode Dept *",
	"Nama Departemen",
	"Kode Jabatan",
	"Nama Jabatan",
	"Kode Area",
	"Nama Area",
	"Tgl Masuk (YYYY-MM-DD)",
	"Email",
	"No HP",
}

// EmployeeImportRow menampung 1 baris data dari file Excel.
type EmployeeImportRow struct {
	RowNum    int
	EmpCode   string
	FirstName string
	LastName  string
	DeptCode  string
	DeptName  string
	PosCode   string
	PosName   string
	AreaCode  string
	AreaName  string
	HireDate  string
	Email     string
	Mobile    string
}

// PendingEmployeeImport adalah baris yang lolos validasi dan siap dikirim ke BioTime.
type PendingEmployeeImport struct {
	Row EmployeeImportRow
	Req models.CreateEmployeeRequest
}

// ImportRowResult adalah hasil per baris untuk ditampilkan di UI.
type ImportRowResult struct {
	RowNum   int    `json:"row"`
	EmpCode  string `json:"emp_code"`
	FullName string `json:"full_name"`
	Status   string `json:"status"` // sukses | duplikat | gagal
	Message  string `json:"message"`
}

// BuildEmployeeTemplate membuat file .xlsx template import beserta sheet referensi.
func BuildEmployeeTemplate(depts []models.BioTimeDepartment, positions []models.BioTimePosition, areas []models.BioTimeArea) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Pegawai"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, err
	}

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1F4E5F"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Color: "B0BEC5", Style: 1},
			{Type: "right", Color: "B0BEC5", Style: 1},
			{Type: "top", Color: "B0BEC5", Style: 1},
			{Type: "bottom", Color: "B0BEC5", Style: 1},
		},
	})
	if err != nil {
		return nil, err
	}
	exampleStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Color: "616161", Italic: true, Size: 11},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"F2F6F7"}, Pattern: 1},
	})
	if err != nil {
		return nil, err
	}

	widths := []float64{20, 22, 22, 14, 26, 14, 26, 12, 22, 22, 26, 18}
	for i, h := range EmployeeImportHeaders {
		col, _ := excelize.ColumnNumberToName(i + 1)
		if err := f.SetCellValue(sheet, col+"1", h); err != nil {
			return nil, err
		}
		if err := f.SetColWidth(sheet, col, col, widths[i]); err != nil {
			return nil, err
		}
	}
	lastCol, _ := excelize.ColumnNumberToName(len(EmployeeImportHeaders))
	if err := f.SetCellStyle(sheet, "A1", lastCol+"1", headerStyle); err != nil {
		return nil, err
	}
	f.SetRowHeight(sheet, 1, 30)

	// Baris contoh (dihapus/diisi ulang oleh operator).
	examples := [][]string{
		{"199001012015031001", "Budi", "Santoso", "B001", "Sekretariat", "J001", "Kepala Bagian", "L001", "Kantor Dinas", "2015-03-01", "budi@barutkab.go.id", "081234567890"},
		{"199205152019022002", "Siti", "Aminah", "B002", "Bidang Bina Marga", "", "", "L001", "Kantor Dinas", "2019-02-02", "", "082345678901"},
	}
	for r, ex := range examples {
		for i, v := range ex {
			col, _ := excelize.ColumnNumberToName(i + 1)
			if err := f.SetCellValue(sheet, fmt.Sprintf("%s%d", col, r+2), v); err != nil {
				return nil, err
			}
		}
		if err := f.SetCellStyle(sheet, fmt.Sprintf("A%d", r+2), fmt.Sprintf("%s%d", lastCol, r+2), exampleStyle); err != nil {
			return nil, err
		}
	}

	_ = f.AutoFilter(sheet, "A1:"+lastCol+"1", nil)
	_ = f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "topLeft",
		Selection: []excelize.Selection{{SQRef: "A2", ActiveCell: "A2", Pane: "topLeft"}},
	})

	writeRefTable(f, "Referensi", depts, positions, areas)
	writeImportGuide(f)

	idx, err := f.GetSheetIndex(sheet)
	if err != nil {
		return nil, err
	}
	f.SetActiveSheet(idx)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeRefTable(f *excelize.File, sheet string, depts []models.BioTimeDepartment, positions []models.BioTimePosition, areas []models.BioTimeArea) {
	if _, err := f.NewSheet(sheet); err != nil {
		return
	}
	titleStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 12, Color: "1F4E5F"}})
	headStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"607D8B"}, Pattern: 1}})

	row := 1
	block := func(title string, headers []string, records [][]string) {
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), title)
		_ = f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), titleStyle)
		row++
		for i, h := range headers {
			col, _ := excelize.ColumnNumberToName(i + 1)
			_ = f.SetCellValue(sheet, fmt.Sprintf("%s%d", col, row), h)
			_ = f.SetCellStyle(sheet, fmt.Sprintf("%s%d", col, row), fmt.Sprintf("%s%d", col, row), headStyle)
		}
		row++
		for _, rec := range records {
			for i, v := range rec {
				col, _ := excelize.ColumnNumberToName(i + 1)
				_ = f.SetCellValue(sheet, fmt.Sprintf("%s%d", col, row), v)
			}
			row++
		}
		row++
	}

	deptRecs := make([][]string, 0, len(depts))
	for _, d := range depts {
		deptRecs = append(deptRecs, []string{d.DeptCode, d.DeptName})
	}
	block("DEPARTEMEN / UNIT KERJA (salin Kode atau Nama ke sheet Pegawai)", []string{"Kode", "Nama"}, deptRecs)

	posRecs := make([][]string, 0, len(positions))
	for _, p := range positions {
		posRecs = append(posRecs, []string{p.PositionCode, p.PositionName})
	}
	block("JABATAN (opsional — kosongkan bila belum ada)", []string{"Kode", "Nama"}, posRecs)

	areaRecs := make([][]string, 0, len(areas))
	for _, a := range areas {
		areaRecs = append(areaRecs, []string{a.AreaCode, a.AreaName})
	}
	block("AREA / LOKASI MESIN (opsional — boleh lebih dari 1, pisahkan koma)", []string{"Kode", "Nama"}, areaRecs)

	_ = f.SetColWidth(sheet, "A", "A", 20)
	_ = f.SetColWidth(sheet, "B", "B", 40)
}

func writeImportGuide(f *excelize.File) {
	sheet := "Petunjuk"
	if _, err := f.NewSheet(sheet); err != nil {
		return
	}
	lines := []string{
		"CARA PAKAI TEMPLATE IMPORT PEGAWAI (MAPEL-ASN)",
		"",
		"1. Isi sheet Pegawai mulai baris 2. Hapus baris contoh bila tidak dipakai.",
		"2. Kolom bertanda * wajib: NIP, Nama Depan, dan Kode Dept (atau Nama Departemen).",
		"3. Departemen / Jabatan / Area: isi Kode saja cukup. Bila Kode kosong, dicocokkan via Nama.",
		"   Bila Kode dan Nama diisi tapi menunjuk data berbeda, baris DITOLAK.",
		"4. Daftar Kode + Nama yang valid ada di sheet Referensi (diambil live dari BioTime).",
		"5. Area boleh lebih dari 1, pisahkan dengan koma (contoh: L001, L002).",
		"6. Tgl Masuk format YYYY-MM-DD (contoh: 2015-03-01). Boleh dikosongkan.",
		"7. NIP yang sudah ada di BioTime DILEWATI (tidak di-update, tidak dobel).",
		"8. Maksimal 1000 baris per file. Simpan sebagai .xlsx lalu upload via tombol Import Excel.",
		"9. Setelah import, enroll wajah tiap pegawai dari device / BioTime, lalu Transfer to Device.",
	}
	for i, l := range lines {
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", i+1), l)
	}
	_ = f.SetColWidth(sheet, "A", "A", 100)
	titleStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 13, Color: "1F4E5F"}})
	_ = f.SetCellStyle(sheet, "A1", "A1", titleStyle)
}

// ParseEmployeeImportXLSX membaca file upload menjadi baris-baris data.
func ParseEmployeeImportXLSX(data []byte) ([]EmployeeImportRow, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("file bukan Excel valid (.xlsx): %v", err)
	}
	defer f.Close()

	sheet := "Pegawai"
	rows, err := f.GetRows(sheet)
	if err != nil {
		sheet = f.GetSheetName(0)
		rows, err = f.GetRows(sheet)
		if err != nil {
			return nil, fmt.Errorf("gagal membaca sheet: %v", err)
		}
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("file kosong — isi data mulai baris 2 sesuai template")
	}

	colIdx := mapHeaderCols(rows[0])
	for _, need := range []string{"empcode", "firstname", "dept"} {
		if _, ok := colIdx[need]; !ok {
			return nil, fmt.Errorf("kolom wajib tidak ditemukan — pakai template resmi via tombol Download Template")
		}
	}

	var out []EmployeeImportRow
	for i, r := range rows[1:] {
		cell := func(key string) string {
			idx, ok := colIdx[key]
			if !ok || idx >= len(r) {
				return ""
			}
			return strings.TrimSpace(r[idx])
		}
		row := EmployeeImportRow{
			RowNum: i + 2, EmpCode: cell("empcode"), FirstName: cell("firstname"),
			LastName: cell("lastname"), DeptCode: cell("deptcode"), DeptName: cell("deptname"),
			PosCode: cell("poscode"), PosName: cell("posname"),
			AreaCode: cell("areacode"), AreaName: cell("areaname"),
			HireDate: cell("hiredate"), Email: cell("email"), Mobile: cell("mobile"),
		}
		if row.EmpCode == "" && row.FirstName == "" && row.LastName == "" && row.DeptCode == "" && row.DeptName == "" {
			continue // baris kosong total, lewati
		}
		out = append(out, row)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tidak ada data — isi data mulai baris 2 sesuai template")
	}
	if len(out) > EmployeeImportMaxRows {
		return nil, fmt.Errorf("maksimal %d baris per file (file ini %d) — pecah jadi beberapa file", EmployeeImportMaxRows, len(out))
	}
	return out, nil
}

// mapHeaderCols memetakan header (ID/EN, toleran tanda *) ke kolom data.
func mapHeaderCols(header []string) map[string]int {
	alias := map[string]string{
		"nip": "empcode", "nip / id pegawai": "empcode", "emp_code": "empcode", "id pegawai": "empcode",
		"nama depan": "firstname", "first_name": "firstname", "firstname": "firstname", "nama": "firstname",
		"nama belakang": "lastname", "last_name": "lastname", "lastname": "lastname",
		"kode dept": "deptcode", "kode departemen": "deptcode", "dept_code": "deptcode", "kode unit kerja": "deptcode", "kode unit": "deptcode", "kode bagian": "deptcode",
		"nama departemen": "deptname", "departemen": "deptname", "unit kerja": "deptname", "bagian": "deptname", "dept": "deptname",
		"kode jabatan": "poscode", "position_code": "poscode", "kode posisi": "poscode",
		"nama jabatan": "posname", "jabatan": "posname", "posisi": "posname", "position": "posname",
		"kode area": "areacode", "area_code": "areacode", "kode lokasi": "areacode",
		"nama area": "areaname", "area": "areaname", "lokasi": "areaname",
		"tgl masuk": "hiredate", "tanggal masuk": "hiredate", "hire_date": "hiredate", "tmt": "hiredate",
		"email": "email",
		"no hp": "mobile", "hp": "mobile", "mobile": "mobile", "no. hp": "mobile", "telepon": "mobile", "no telepon": "mobile",
	}
	m := map[string]int{}
	for i, h := range header {
		key := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(h), "*")))
		if canon, ok := alias[key]; ok {
			if _, dup := m[canon]; !dup {
				m[canon] = i
			}
		}
	}
	if _, ok := m["deptcode"]; ok {
		m["dept"] = m["deptcode"]
	} else if _, ok := m["deptname"]; ok {
		m["dept"] = m["deptname"]
	}
	return m
}

// ValidateImportRows memvalidasi baris + me-resolve relasi (kode prioritas, nama fallback).
// existingCodes berisi emp_code yang sudah ada di BioTime (key dinormalisasi uppercase).
func ValidateImportRows(rows []EmployeeImportRow, depts []models.BioTimeDepartment, positions []models.BioTimePosition, areas []models.BioTimeArea, existingCodes map[string]bool) ([]PendingEmployeeImport, []ImportRowResult) {
	deptIDByCode, deptIDByName := map[string]int64{}, map[string]int64{}
	for _, d := range depts {
		deptIDByCode[strings.ToUpper(strings.TrimSpace(d.DeptCode))] = d.ID
		deptIDByName[strings.ToUpper(strings.TrimSpace(d.DeptName))] = d.ID
	}
	posIDByCode, posIDByName := map[string]int64{}, map[string]int64{}
	for _, p := range positions {
		posIDByCode[strings.ToUpper(strings.TrimSpace(p.PositionCode))] = p.ID
		posIDByName[strings.ToUpper(strings.TrimSpace(p.PositionName))] = p.ID
	}
	areaByCode, areaByName := map[string]models.BioTimeArea{}, map[string]models.BioTimeArea{}
	for _, a := range areas {
		areaByCode[strings.ToUpper(strings.TrimSpace(a.AreaCode))] = a
		areaByName[strings.ToUpper(strings.TrimSpace(a.AreaName))] = a
	}

	seen := map[string]bool{}
	var pending []PendingEmployeeImport
	var results []ImportRowResult
	fail := func(r EmployeeImportRow, msg string) {
		results = append(results, ImportRowResult{RowNum: r.RowNum, EmpCode: r.EmpCode, FullName: strings.TrimSpace(r.FirstName + " " + r.LastName), Status: "gagal", Message: msg})
	}

	for _, r := range rows {
		name := strings.TrimSpace(r.FirstName + " " + r.LastName)
		if r.EmpCode == "" {
			fail(r, "NIP kosong")
			continue
		}
		if r.FirstName == "" {
			fail(r, "Nama Depan kosong")
			continue
		}
		normCode := strings.ToUpper(strings.TrimSpace(r.EmpCode))
		if existingCodes[normCode] {
			results = append(results, ImportRowResult{RowNum: r.RowNum, EmpCode: r.EmpCode, FullName: name, Status: "duplikat", Message: "NIP sudah ada di BioTime — dilewati"})
			continue
		}
		if seen[normCode] {
			results = append(results, ImportRowResult{RowNum: r.RowNum, EmpCode: r.EmpCode, FullName: name, Status: "duplikat", Message: "NIP dobel di file ini — baris pertama yang dipakai"})
			continue
		}

		deptID, ok := resolvePaired(r.DeptCode, r.DeptName, deptIDByCode, deptIDByName)
		if !ok {
			fail(r, deptErrMsg(r.DeptCode, r.DeptName, "Departemen"))
			continue
		}

		var posID *int64
		if r.PosCode != "" || r.PosName != "" {
			id, ok := resolvePaired(r.PosCode, r.PosName, posIDByCode, posIDByName)
			if !ok {
				fail(r, deptErrMsg(r.PosCode, r.PosName, "Jabatan"))
				continue
			}
			posID = &id
		}

		var areaIDs []int64
		if r.AreaCode != "" || r.AreaName != "" {
			ids, msg := resolveMulti(r.AreaCode, r.AreaName, areaByCode, areaByName)
			if msg != "" {
				fail(r, "Area: "+msg)
				continue
			}
			areaIDs = ids
		}
		if len(areaIDs) == 0 {
			areaIDs = []int64{1}
		}

		hire := ""
		if r.HireDate != "" {
			t, err := parseImportDate(r.HireDate)
			if err != nil {
				fail(r, "Tgl Masuk tidak dikenal — pakai format YYYY-MM-DD")
				continue
			}
			hire = t.Format("2006-01-02")
		}

		if r.Email != "" && !strings.Contains(r.Email, "@") {
			fail(r, "Email tidak valid")
			continue
		}

		seen[normCode] = true
		req := models.CreateEmployeeRequest{
			EmpCode: strings.TrimSpace(r.EmpCode), FirstName: strings.TrimSpace(r.FirstName),
			Department: deptID, Area: areaIDs, Position: posID,
			HireDate: hire, Email: strings.TrimSpace(r.Email), Mobile: strings.TrimSpace(r.Mobile),
		}
		if r.LastName != "" {
			req.LastName = strings.TrimSpace(r.LastName)
		}
		pending = append(pending, PendingEmployeeImport{Row: r, Req: req})
	}
	return pending, results
}

func deptErrMsg(code, name, label string) string {
	switch {
	case code != "" && name != "":
		return fmt.Sprintf("%s: Kode %q dan Nama %q menunjuk data berbeda — samakan via sheet Referensi", label, code, name)
	case code != "":
		return fmt.Sprintf("%s: Kode %q tidak ada di BioTime — cek sheet Referensi", label, code)
	case name != "":
		return fmt.Sprintf("%s: Nama %q tidak ada di BioTime — cek sheet Referensi", label, name)
	default:
		return fmt.Sprintf("%s wajib diisi (Kode atau Nama)", label)
	}
}

// resolvePaired mencocokkan kode-nama: kode prioritas, nama fallback, keduanya harus sinkron.
func resolvePaired(code, name string, byCode, byName map[string]int64) (int64, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	name = strings.ToUpper(strings.TrimSpace(name))
	if code == "" && name == "" {
		return 0, false
	}
	if code != "" {
		id, ok := byCode[code]
		if !ok {
			return 0, false
		}
		if name != "" {
			nid, ok := byName[name]
			if !ok || nid != id {
				return 0, false
			}
		}
		return id, true
	}
	id, ok := byName[name]
	if !ok {
		return 0, false
	}
	return id, true
}

// resolveMulti seperti resolvePaired tapi mendukung daftar koma (untuk Area).
func resolveMulti(code, name string, byCode map[string]models.BioTimeArea, byName map[string]models.BioTimeArea) ([]int64, string) {
	seen := map[int64]bool{}
	var ids []int64
	add := func(v models.BioTimeArea) {
		if !seen[v.ID] {
			seen[v.ID] = true
			ids = append(ids, v.ID)
		}
	}
	if strings.TrimSpace(code) != "" {
		for _, tok := range strings.Split(code, ",") {
			tok = strings.ToUpper(strings.TrimSpace(tok))
			if tok == "" {
				continue
			}
			v, ok := byCode[tok]
			if !ok {
				return nil, fmt.Sprintf("Kode %q tidak ada di BioTime", tok)
			}
			add(v)
		}
	}
	if strings.TrimSpace(name) != "" {
		for _, tok := range strings.Split(name, ",") {
			tok = strings.ToUpper(strings.TrimSpace(tok))
			if tok == "" {
				continue
			}
			v, ok := byName[tok]
			if !ok {
				return nil, fmt.Sprintf("Nama %q tidak ada di BioTime", tok)
			}
			add(v)
		}
	}
	return ids, ""
}

func parseImportDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02", "02/01/2006", "02-01-2006", "2006/01/02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("format tanggal tidak dikenal")
}
