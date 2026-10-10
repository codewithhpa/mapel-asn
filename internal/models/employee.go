package models

import (
	"encoding/json"
	"fmt"
)

// BioTimeDepartment merepresentasikan unit kerja / bagian pada BioTime 8.5
type BioTimeDepartment struct {
	ID             int64   `json:"id"`
	DeptCode       string  `json:"dept_code"`
	DeptName       string  `json:"dept_name"`
	ParentDept     any     `json:"parent_dept,omitempty"`
	ParentDeptName *string `json:"parent_dept_name,omitempty"`
}

// DepartmentListResponse pembungkus respon GET /personnel/api/departments/
type DepartmentListResponse struct {
	Count    int                 `json:"count"`
	Next     *string             `json:"next"`
	Previous *string             `json:"previous"`
	Msg      string              `json:"msg"`
	Code     int                 `json:"code"`
	Data     []BioTimeDepartment `json:"data"`
}

// BioTimeArea merepresentasikan lokasi kerja / area mesin Face Recognition
type BioTimeArea struct {
	ID             int64   `json:"id"`
	AreaCode       string  `json:"area_code"`
	AreaName       string  `json:"area_name"`
	ParentArea     any     `json:"parent_area,omitempty"`
	ParentAreaName *string `json:"parent_area_name,omitempty"`
}

// AreaListResponse pembungkus respon GET /personnel/api/areas/
type AreaListResponse struct {
	Count    int           `json:"count"`
	Next     *string       `json:"next"`
	Previous *string       `json:"previous"`
	Msg      string        `json:"msg"`
	Code     int           `json:"code"`
	Data     []BioTimeArea `json:"data"`
}

// BioTimePosition merepresentasikan data jabatan kedinasan pada BioTime
type BioTimePosition struct {
	ID                 int64   `json:"id"`
	PositionCode       string  `json:"position_code"`
	PositionName       string  `json:"position_name"`
	ParentPosition     any     `json:"parent_position,omitempty"`
	ParentPositionName *string `json:"parent_position_name,omitempty"`
}

// PositionListResponse pembungkus respon GET /personnel/api/positions/
type PositionListResponse struct {
	Count    int               `json:"count"`
	Next     *string           `json:"next"`
	Previous *string           `json:"previous"`
	Msg      string            `json:"msg"`
	Code     int               `json:"code"`
	Data     []BioTimePosition `json:"data"`
}

// BioTimeResign merepresentasikan data mutasi ASN / penonaktifan presensi
type BioTimeResign struct {
	ID         int64          `json:"id"`
	ResignDate string         `json:"resign_date"`
	ResignType int            `json:"resign_type"` // 1 = Resign / Mutasi
	DisableAtt bool           `json:"disableatt"`
	Reason     string         `json:"reason,omitempty"`
	Employee   *ResignEmpInfo `json:"employee,omitempty"`
}

type ResignEmpInfo struct {
	ID        int64  `json:"id"`
	EmpCode   string `json:"emp_code"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
}

// ResignListResponse pembungkus respon GET /personnel/api/resigns/
type ResignListResponse struct {
	Count    int             `json:"count"`
	Next     *string         `json:"next"`
	Previous *string         `json:"previous"`
	Msg      string          `json:"msg"`
	Code     int             `json:"code"`
	Data     []BioTimeResign `json:"data"`
}

// BioTimeEmployee merepresentasikan 1 entitas pegawai ASN di BioTime 8.5
type BioTimeEmployee struct {
	ID         int64   `json:"id"`
	EmpCode    string  `json:"emp_code"`
	FirstName  string  `json:"first_name"`
	LastName   *string `json:"last_name,omitempty"`
	FullName   string  `json:"full_name,omitempty"`
	FormatName string  `json:"format_name,omitempty"`
	Photo      *string `json:"photo,omitempty"`
	CardNo     *string `json:"card_no,omitempty"`
	HireDate   *string `json:"hire_date,omitempty"`
	Gender     *string `json:"gender,omitempty"`
	Birthday   *string `json:"birthday,omitempty"`
	ContactTel *string `json:"contact_tel,omitempty"`
	Mobile     *string `json:"mobile,omitempty"`
	Email      *string `json:"email,omitempty"`
	EnrollSN   string  `json:"enroll_sn,omitempty"`
	VLFace     string  `json:"vl_face,omitempty"`
	UpdateTime string  `json:"update_time,omitempty"`
	AppStatus  int     `json:"app_status,omitempty"`

	// Parsing fleksibel Department (bisa objek saat GET, bisa int saat detail/payload)
	DepartmentRaw  json.RawMessage `json:"department,omitempty"`
	DepartmentID   int64           `json:"-"`
	DepartmentName string          `json:"-"`

	// Parsing fleksibel Area (bisa array of objects saat GET, bisa array of int saat POST)
	AreaRaw   json.RawMessage `json:"area,omitempty"`
	AreaIDs   []int64         `json:"-"`
	AreaNames []string        `json:"area_names,omitempty"`

	// Parsing fleksibel Position
	PositionRaw  json.RawMessage `json:"position,omitempty"`
	PositionID   int64           `json:"-"`
	PositionName string          `json:"-"`
}

// ParseHelpers mengekstrak ID dan Nama dari raw JSON Department, Area, dan Position.
// Idempoten: aman dipanggil ulang (hasil sebelumnya dibuang dulu).
func (e *BioTimeEmployee) ParseHelpers() {
	e.AreaIDs = nil
	e.AreaNames = nil
	// Parse Department
	if len(e.DepartmentRaw) > 0 {
		var deptObj BioTimeDepartment
		if err := json.Unmarshal(e.DepartmentRaw, &deptObj); err == nil && deptObj.ID > 0 {
			e.DepartmentID = deptObj.ID
			e.DepartmentName = deptObj.DeptName
		} else {
			var deptID int64
			if err := json.Unmarshal(e.DepartmentRaw, &deptID); err == nil {
				e.DepartmentID = deptID
			}
		}
	}

	// Parse Area
	if len(e.AreaRaw) > 0 {
		var areaObjs []BioTimeArea
		if err := json.Unmarshal(e.AreaRaw, &areaObjs); err == nil {
			for _, a := range areaObjs {
				e.AreaIDs = append(e.AreaIDs, a.ID)
				e.AreaNames = append(e.AreaNames, a.AreaName)
			}
		} else {
			var areaIDs []int64
			if err := json.Unmarshal(e.AreaRaw, &areaIDs); err == nil {
				e.AreaIDs = areaIDs
			}
		}
	}

	// Parse Position
	if len(e.PositionRaw) > 0 {
		var posObj BioTimePosition
		if err := json.Unmarshal(e.PositionRaw, &posObj); err == nil && posObj.ID > 0 {
			e.PositionID = posObj.ID
			e.PositionName = posObj.PositionName
		} else {
			var posID int64
			if err := json.Unmarshal(e.PositionRaw, &posID); err == nil {
				e.PositionID = posID
			}
		}
	}

	// Normalisasi Nama Lengkap
	if e.FullName == "" {
		if e.LastName != nil && *e.LastName != "" {
			e.FullName = fmt.Sprintf("%s %s", e.FirstName, *e.LastName)
		} else {
			e.FullName = e.FirstName
		}
	}
}

// EmployeeListResponse pembungkus respon GET /personnel/api/employees/
type EmployeeListResponse struct {
	Count    int               `json:"count"`
	Next     *string           `json:"next"`
	Previous *string           `json:"previous"`
	Msg      string            `json:"msg"`
	Code     int               `json:"code"`
	Data     []BioTimeEmployee `json:"data"`
}

// Struct Request Payload untuk CRUD

type CreateEmployeeRequest struct {
	EmpCode    string  `json:"emp_code"`
	FirstName  string  `json:"first_name"`
	LastName   string  `json:"last_name,omitempty"`
	Department int64   `json:"department"`
	Area       []int64 `json:"area"`
	Position   *int64  `json:"position,omitempty"`
	HireDate   string  `json:"hire_date,omitempty"`
	Email      string  `json:"email,omitempty"`
	Mobile     string  `json:"mobile,omitempty"`
}

type UpdateEmployeeRequest struct {
	EmpCode    string  `json:"emp_code,omitempty"`
	FirstName  string  `json:"first_name,omitempty"`
	LastName   string  `json:"last_name,omitempty"`
	Department int64   `json:"department,omitempty"`
	Area       []int64 `json:"area,omitempty"`
	Position   *int64  `json:"position,omitempty"`
	HireDate   string  `json:"hire_date,omitempty"`
	Email      string  `json:"email,omitempty"`
	Mobile     string  `json:"mobile,omitempty"`
}

type CreateDepartmentRequest struct {
	DeptCode   string `json:"dept_code"`
	DeptName   string `json:"dept_name"`
	ParentDept *int64 `json:"parent_dept,omitempty"`
}

type UpdateDepartmentRequest struct {
	DeptName   string `json:"dept_name,omitempty"`
	ParentDept *int64 `json:"parent_dept,omitempty"`
}

type CreateAreaRequest struct {
	AreaCode   string `json:"area_code"`
	AreaName   string `json:"area_name"`
	ParentArea *int64 `json:"parent_area,omitempty"`
}

type UpdateAreaRequest struct {
	AreaName   string `json:"area_name,omitempty"`
	ParentArea *int64 `json:"parent_area,omitempty"`
}

type CreatePositionRequest struct {
	PositionCode   string `json:"position_code"`
	PositionName   string `json:"position_name"`
	ParentPosition *int64 `json:"parent_position,omitempty"`
}

type UpdatePositionRequest struct {
	PositionName   string `json:"position_name,omitempty"`
	ParentPosition *int64 `json:"parent_position,omitempty"`
}

type CreateResignRequest struct {
	Employee   int64  `json:"employee"`
	ResignDate string `json:"resign_date"`
	ResignType int    `json:"resign_type"`
	DisableAtt bool   `json:"disableatt"`
	Reason     string `json:"reason"`
}

type ReinstatementRequest struct {
	Resigns []int64 `json:"resigns"`
}
