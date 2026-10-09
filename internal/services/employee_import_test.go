package services

import (
	"bytes"
	"testing"

	"dashboard-presensi/internal/models"

	"github.com/xuri/excelize/v2"
)

func buildTestXLSX(t *testing.T, headers []string, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	for i, h := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		if err := f.SetCellValue("Sheet1", col+"1", h); err != nil {
			t.Fatal(err)
		}
	}
	for r, row := range rows {
		for i, v := range row {
			col, _ := excelize.ColumnNumberToName(i + 1)
			if err := f.SetCellValue("Sheet1", col+itoa(r+2), v); err != nil {
				t.Fatal(err)
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func testMasterData() ([]models.BioTimeDepartment, []models.BioTimePosition, []models.BioTimeArea) {
	depts := []models.BioTimeDepartment{
		{ID: 1, DeptCode: "B001", DeptName: "Sekretariat"},
		{ID: 2, DeptCode: "B002", DeptName: "Bidang Bina Marga"},
	}
	positions := []models.BioTimePosition{
		{ID: 10, PositionCode: "J001", PositionName: "Kepala Bagian"},
	}
	areas := []models.BioTimeArea{
		{ID: 1, AreaCode: "L001", AreaName: "Kantor Dinas"},
		{ID: 2, AreaCode: "L002", AreaName: "Lapangan Apel"},
	}
	return depts, positions, areas
}

func TestParseEmployeeImportXLSX(t *testing.T) {
	data := buildTestXLSX(t, EmployeeImportHeaders, [][]string{
		{"199001012015031001", "Budi", "Santoso", "B001", "Sekretariat", "J001", "Kepala Bagian", "L001", "Kantor Dinas", "2015-03-01", "budi@x.go.id", "081234"},
		{"", "", "", "", "", "", "", "", "", "", "", ""},
	})
	rows, err := ParseEmployeeImportXLSX(data)
	if err != nil {
		t.Fatalf("parse gagal: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("mau 1 baris (baris kosong dilewati), dapat %d", len(rows))
	}
	if rows[0].EmpCode != "199001012015031001" || rows[0].RowNum != 2 {
		t.Fatalf("isi baris salah: %+v", rows[0])
	}
}

func TestValidateImportRows(t *testing.T) {
	depts, positions, areas := testMasterData()
	rows := []EmployeeImportRow{
		{RowNum: 2, EmpCode: "NIP001", FirstName: "Budi", DeptCode: "B001", DeptName: "Sekretariat", PosCode: "J001", AreaCode: "L001, L002", HireDate: "2015-03-01"},
		{RowNum: 3, EmpCode: "NIP002", FirstName: "Siti", DeptName: "Bidang Bina Marga"},
		{RowNum: 4, EmpCode: "NIP999", FirstName: "Ada", DeptCode: "B001"},
		{RowNum: 5, EmpCode: "NIP003", FirstName: "", DeptCode: "B001"},
		{RowNum: 6, EmpCode: "NIP004", FirstName: "Ragu", DeptCode: "B001", DeptName: "Bidang Bina Marga"},
		{RowNum: 7, EmpCode: "NIP005", FirstName: "Salah", DeptCode: "B999"},
		{RowNum: 8, EmpCode: "NIP001", FirstName: "Dobel", DeptCode: "B001"},
	}
	existing := map[string]bool{"NIP999": true}

	pending, results := ValidateImportRows(rows, depts, positions, areas, existing)

	if len(pending) != 2 {
		t.Fatalf("mau 2 pending, dapat %d", len(pending))
	}
	if pending[0].Req.Department != 1 || pending[0].Req.Position == nil || *pending[0].Req.Position != 10 {
		t.Fatalf("resolve baris 2 salah: %+v", pending[0].Req)
	}
	if len(pending[0].Req.Area) != 2 {
		t.Fatalf("area multi-kode baris 2 harus 2 ID, dapat %v", pending[0].Req.Area)
	}
	if pending[1].Req.Department != 2 {
		t.Fatalf("fallback nama dept baris 3 harus ID 2, dapat %d", pending[1].Req.Department)
	}
	if len(results) != 5 {
		t.Fatalf("mau 5 hasil non-pending, dapat %d: %+v", len(results), results)
	}
	statusByRow := map[int]string{}
	for _, r := range results {
		statusByRow[r.RowNum] = r.Status
	}
	for row, want := range map[int]string{4: "duplikat", 5: "gagal", 6: "gagal", 7: "gagal", 8: "duplikat"} {
		if statusByRow[row] != want {
			t.Fatalf("baris %d: mau %q, dapat %q", row, want, statusByRow[row])
		}
	}
}

func TestBuildEmployeeTemplate(t *testing.T) {
	depts, positions, areas := testMasterData()
	data, err := BuildEmployeeTemplate(depts, positions, areas)
	if err != nil {
		t.Fatalf("build template gagal: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("template kosong")
	}
	rows, err := ParseEmployeeImportXLSX(data)
	if err != nil {
		t.Fatalf("template hasil generate harus bisa di-parse balik: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("template harus memuat 2 baris contoh, dapat %d", len(rows))
	}
}
