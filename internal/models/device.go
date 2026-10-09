package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// BioTimeTerminal merepresentasikan 1 unit device Face Recognition di BioTime.
// BioTime mengirim sebagian angka sebagai string (mis. "state":"1"), jadi pakai FlexInt.
type BioTimeTerminal struct {
	ID               int64           `json:"id"`
	SN               string          `json:"sn"`
	Alias            string          `json:"alias"`
	TerminalName     string          `json:"terminal_name"`
	IPAddress        string          `json:"ip_address"`
	AreaName         string          `json:"area_name"`
	State            FlexInt         `json:"state"` // 1 = online
	LastActivity     string          `json:"last_activity"`
	UserCount        FlexInt         `json:"user_count"`
	FaceCount        FlexInt         `json:"face_count"`
	FPCount          FlexInt         `json:"fp_count"`
	PalmCount        FlexInt         `json:"palm_count"`
	TransactionCount FlexInt         `json:"transaction_count"`
	IsAttendance     FlexInt         `json:"is_attendance"`
	TransferInterval FlexInt         `json:"transfer_interval"`
	AreaRaw          json.RawMessage `json:"area,omitempty"`
}

// FlexInt menerima angka JSON maupun string berisi angka ("1") atau null.
type FlexInt int

func (f *FlexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("FlexInt: nilai %q bukan angka", s)
	}
	*f = FlexInt(n)
	return nil
}

// IsOnline true bila device terakhir terhubung normal.
func (t BioTimeTerminal) IsOnline() bool { return t.State == 1 }

// DisplayName nama tampilan device (alias diprioritaskan).
func (t BioTimeTerminal) DisplayName() string {
	if t.Alias != "" {
		return t.Alias
	}
	return t.TerminalName
}

// TerminalListResponse pembungkus GET /iclock/api/terminals/
type TerminalListResponse struct {
	Count    int               `json:"count"`
	Next     *string           `json:"next"`
	Previous *string           `json:"previous"`
	Msg      string            `json:"msg"`
	Code     int               `json:"code"`
	Data     []BioTimeTerminal `json:"data"`
}

// TerminalActionDetail respons aksi device (upload / resync / reboot).
type TerminalActionDetail struct {
	Detail string `json:"detail"`
}

// TerminalIDsRequest body aksi per device.
type TerminalIDsRequest struct {
	TerminalIDs []int64 `json:"terminal_ids"`
}

// EmployeeIDsRequest body sync pegawai ke device.
type EmployeeIDsRequest struct {
	EmployeeIDs []int64 `json:"employee_ids"`
}
