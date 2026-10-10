package services

import (
	"context"
	"fmt"

	"dashboard-presensi/internal/models"
)

// FetchTerminals menarik daftar device dari /iclock/api/terminals/
func (s *BioTimeClient) FetchTerminals(ctx context.Context) ([]models.BioTimeTerminal, error) {
	queryParams := map[string]string{
		"page_size": "200",
	}
	var termResp models.TerminalListResponse
	resp, err := s.executeRequest(ctx, "GET", "/iclock/api/terminals/", nil, queryParams, &termResp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("fetch terminals gagal status %d: %s", resp.StatusCode(), resp.String())
	}
	return termResp.Data, nil
}

// UploadTransactions memaksa device upload ulang tap tertahan.
// Setara tombol BioTime: Data Transfer -> Upload Transaction.
func (s *BioTimeClient) UploadTransactions(ctx context.Context, terminalIDs []int64) error {
	var out models.TerminalActionDetail
	resp, err := s.executeRequest(ctx, "POST", "/iclock/api/terminals/upload_transaction/",
		map[string]any{"terminals": terminalIDs}, nil, &out)
	if err != nil {
		return err
	}
	if resp.StatusCode() != 200 {
		return fmt.Errorf("upload transaction gagal status %d: %s", resp.StatusCode(), resp.String())
	}
	return nil
}

// UploadUsersFromDevice memaksa device upload data user + template biometrik ke server.
// Setara tombol BioTime: Data Transfer -> Upload User Data.
// PERHATIAN: bila PIN user di device tidak sama dengan emp_code di server,
// BioTime membuat record pegawai BARU (duplikat) alih-alih menggabungkan.
func (s *BioTimeClient) UploadUsersFromDevice(ctx context.Context, terminalIDs []int64) error {
	var out models.TerminalActionDetail
	resp, err := s.executeRequest(ctx, "POST", "/iclock/api/terminals/upload_all/",
		map[string]any{"terminals": terminalIDs}, nil, &out)
	if err != nil {
		return err
	}
	if resp.StatusCode() != 200 {
		return fmt.Errorf("upload user data gagal status %d: %s", resp.StatusCode(), resp.String())
	}
	return nil
}

// ResyncEmployeesToDevice menandai pegawai agar datanya di-sync ulang ke device.
// Setara tombol BioTime: Data Transfer -> Sync Data to the Device.
func (s *BioTimeClient) ResyncEmployeesToDevice(ctx context.Context, employeeIDs []int64) error {
	var out models.TerminalActionDetail
	resp, err := s.executeRequest(ctx, "POST", "/personnel/api/employees/resync_to_device/",
		map[string]any{"employees": employeeIDs}, nil, &out)
	if err != nil {
		return err
	}
	if resp.StatusCode() != 200 {
		return fmt.Errorf("resync ke device gagal status %d: %s", resp.StatusCode(), resp.String())
	}
	return nil
}
