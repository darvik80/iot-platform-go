package services

import (
	"os"
	"strings"
	"testing"

	"cogitor-iot-platform/internal/config"
)

func TestStoreFirmwareFile(t *testing.T) {
	p := newTestPlatform(t, config.Config{
		Platform: config.PlatformConfig{PublicBaseURL: "http://192.168.1.10:8080"},
	})

	pr, err := p.CreateProduct(1, ProductInput{ProductID: "lamp", Name: "Lamp"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	fw, err := p.StoreFirmwareFile(FirmwareInput{
		ProductID: pr.ProductID,
		Version:   "1.2.0",
		Module:    "main",
	}, strings.NewReader("hello-bin"))
	if err != nil {
		t.Fatalf("StoreFirmwareFile: %v", err)
	}
	if fw.Size != 9 {
		t.Fatalf("size=%d want 9", fw.Size)
	}
	if fw.SHA256 == "" || !fw.Hosted || fw.DownloadToken == "" {
		t.Fatalf("hosted fields missing: %+v", fw)
	}
	wantURL := "http://192.168.1.10:8080/ota/fw/" + fw.DownloadToken + ".bin"
	if fw.URL != wantURL {
		t.Fatalf("url=%q want %q", fw.URL, wantURL)
	}

	got, path, err := p.HostedFirmware(fw.DownloadToken + ".bin")
	if err != nil {
		t.Fatalf("HostedFirmware: %v", err)
	}
	if got.ID != fw.ID {
		t.Fatalf("id=%d want %d", got.ID, fw.ID)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "hello-bin" {
		t.Fatalf("file=%q", data)
	}

	listed, err := p.ListFirmware(0, "lamp")
	if err != nil {
		t.Fatalf("ListFirmware: %v", err)
	}
	if len(listed) != 1 || !listed[0].Hosted || listed[0].URL != wantURL {
		t.Fatalf("list=%+v", listed)
	}

	byID, err := p.firmwareByID(fw.ID)
	if err != nil {
		t.Fatalf("firmwareByID: %v", err)
	}
	if byID.URL != wantURL {
		t.Fatalf("firmwareByID url=%q want %q", byID.URL, wantURL)
	}

	replaced, err := p.StoreFirmwareFile(FirmwareInput{
		ProductID: pr.ProductID,
		Version:   "1.2.0",
		Module:    "main",
	}, strings.NewReader("hello-bin-v2"))
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if replaced.ID != fw.ID {
		t.Fatalf("replace id=%d want %d", replaced.ID, fw.ID)
	}
	if replaced.Size != 12 {
		t.Fatalf("replace size=%d want 12", replaced.Size)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("old hosted file still exists: %v", err)
	}
	_, newPath, err := p.HostedFirmware(replaced.DownloadToken)
	if err != nil {
		t.Fatalf("HostedFirmware after replace: %v", err)
	}
	data, err = os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read replaced file: %v", err)
	}
	if string(data) != "hello-bin-v2" {
		t.Fatalf("replaced file=%q", data)
	}
	listed, err = p.ListFirmware(0, "lamp")
	if err != nil {
		t.Fatalf("ListFirmware after replace: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != fw.ID {
		t.Fatalf("expected one firmware after replace, got %+v", listed)
	}

	if err := p.DeleteFirmware(fw.ID); err != nil {
		t.Fatalf("DeleteFirmware: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("hosted file still exists: %v", err)
	}
	listed, err = p.ListFirmware(0, "lamp")
	if err != nil {
		t.Fatalf("ListFirmware after delete: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected empty list after delete, got %+v", listed)
	}
	if _, _, err := p.HostedFirmware(fw.DownloadToken); err == nil {
		t.Fatal("expected hosted download to fail after delete")
	}
}

func TestRegisterFirmwareRequiresURL(t *testing.T) {
	p := newTestPlatform(t, config.Config{})
	pr, err := p.CreateProduct(1, ProductInput{ProductID: "lamp", Name: "Lamp"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := p.RegisterFirmware(FirmwareInput{ProductID: pr.ProductID, Version: "1.0.0"}); err == nil {
		t.Fatal("expected url required")
	}
}
