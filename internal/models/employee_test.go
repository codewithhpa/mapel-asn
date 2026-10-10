package models

import (
	"encoding/json"
	"testing"
)

func TestBioTimeEmployee_ParseHelpersIdempotent(t *testing.T) {
	raw := `{"id":1,"emp_code":"NIP001","first_name":"Budi","area":[{"id":4,"area_code":"2","area_name":"DISKOMINFO-Server"},{"id":5,"area_code":"3","area_name":"DISKOMINFO-Office"}]}`
	var e BioTimeEmployee
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("unmarshal gagal: %v", err)
	}
	e.ParseHelpers()
	e.ParseHelpers()
	if len(e.AreaIDs) != 2 || len(e.AreaNames) != 2 {
		t.Fatalf("panggil 2x harus tetap 2 area, dapat %+v", e)
	}
	out, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal gagal: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshal balik gagal: %v", err)
	}
	names, ok := back["area_names"].([]any)
	if !ok || len(names) != 2 || names[0] != "DISKOMINFO-Server" {
		t.Fatalf("area_names harus ikut terserialisasi: %s", out)
	}
}
