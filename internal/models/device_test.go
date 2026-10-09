package models

import (
	"encoding/json"
	"testing"
)

func TestBioTimeTerminal_ParsesRealBioTimeShape(t *testing.T) {
	raw := `{"id":4,"sn":"GAT5261100070","alias":"SpeedFace M4","state":"1","user_count":9,"face_count":6,"is_attendance":1,"transfer_interval":null,"area":{"id":4,"area_code":"2","area_name":"DISKOMINFO-Server"}}`
	var term BioTimeTerminal
	if err := json.Unmarshal([]byte(raw), &term); err != nil {
		t.Fatalf("parse gagal: %v", err)
	}
	if !term.IsOnline() {
		t.Errorf("state \"1\" harus online")
	}
	if term.UserCount != 9 || term.FaceCount != 6 || term.IsAttendance != 1 || term.TransferInterval != 0 {
		t.Errorf("angka salah: %+v", term)
	}
}

func TestFlexInt_AcceptsNumberStringNull(t *testing.T) {
	var v struct {
		A FlexInt `json:"a"`
		B FlexInt `json:"b"`
		C FlexInt `json:"c"`
	}
	if err := json.Unmarshal([]byte(`{"a":2,"b":"3","c":null}`), &v); err != nil {
		t.Fatalf("parse gagal: %v", err)
	}
	if v.A != 2 || v.B != 3 || v.C != 0 {
		t.Fatalf("hasil salah: %+v", v)
	}
}
