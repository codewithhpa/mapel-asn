package models

// LoginRequest merepresentasikan payload untuk POST /jwt-api-token-auth/
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse merepresentasikan respons token JWT dari BioTime 8.5
type LoginResponse struct {
	Token string `json:"token"`
}

// BioTimeTransaction merepresentasikan 1 baris log transaksi Face Recognition
type BioTimeTransaction struct {
	ID                int64  `json:"id"`
	EmpCode           string `json:"emp_code"`
	FirstName         string `json:"first_name"`
	LastName          string `json:"last_name"`
	Department        string `json:"department,omitempty"`
	Position          string `json:"position,omitempty"`
	PunchTime         string `json:"punch_time"` // Format: "YYYY-MM-DD HH:mm:ss"
	PunchState        string `json:"punch_state"`
	PunchStateDisplay string `json:"punch_state_display,omitempty"`
	VerifyType        int    `json:"verify_type,omitempty"`
	VerifyTypeDisplay string `json:"verify_type_display,omitempty"`
	WorkCode          string `json:"work_code,omitempty"`
	TerminalSN        string `json:"terminal_sn,omitempty"`
	TerminalAlias     string `json:"terminal_alias,omitempty"`
	AreaAlias         string `json:"area_alias,omitempty"`
}

// TransactionResponse merepresentasikan pembungkus respons dari /iclock/api/transactions/
type TransactionResponse struct {
	Count    int                  `json:"count"`
	Next     *string              `json:"next"`
	Previous *string              `json:"previous"`
	Msg      string               `json:"msg"`
	Code     int                  `json:"code"`
	Data     []BioTimeTransaction `json:"data"`
}
