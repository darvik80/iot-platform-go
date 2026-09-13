package services

import (
	"path/filepath"
	"testing"

	"cogitor-iot-platform/internal/config"
	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
)

func TestUpdateProductMethods(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	p := New(database, config.Config{}, nil)

	created, err := p.CreateProduct(ProductInput{
		ProductID: "test-product",
		Name:      "Test Product",
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	updated, err := p.UpdateProduct(created.ID, ProductInput{
		SetMethods: true,
		Methods: []models.ProductMethod{
			{Name: "reboot", Label: "Reboot"},
			{Name: "setLed", Label: "Set LED"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}

	got, err := p.GetProduct(updated.ID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}

	byName := map[string]models.ProductMethod{}
	for _, m := range got.Methods {
		byName[m.Name] = m
	}
	if _, ok := byName["reboot"]; !ok {
		t.Fatalf("expected method %q, got methods=%v", "reboot", methodNames(got.Methods))
	}
	if _, ok := byName["setLed"]; !ok {
		t.Fatalf("expected method %q, got methods=%v", "setLed", methodNames(got.Methods))
	}
	if _, ok := byName["rpc-list"]; !ok {
		t.Fatalf("expected built-in method rpc-list, got methods=%v", methodNames(got.Methods))
	}
	if _, ok := byName["rpc_list"]; ok {
		t.Fatalf("rpc_list must not be stored as a separate method, got methods=%v", methodNames(got.Methods))
	}
}

func TestBuiltinRPCListAlwaysPresent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	p := New(database, config.Config{}, nil)
	created, err := p.CreateProduct(ProductInput{
		ProductID:  "rpc-list-product",
		Name:       "RPC List Product",
		SetMethods: true,
		Methods: []models.ProductMethod{
			{Name: "rpc_list", Label: "Should be dropped"},
			{Name: "echo", Label: "Echo"},
		},
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	byName := map[string]models.ProductMethod{}
	for _, m := range created.Methods {
		byName[m.Name] = m
	}
	if m, ok := byName["rpc-list"]; !ok {
		t.Fatalf("expected rpc-list, got %v", methodNames(created.Methods))
	} else if m.Label == "Should be dropped" {
		t.Fatalf("rpc_list alias must not replace rpc-list: %+v", m)
	}
	if _, ok := byName["rpc_list"]; ok {
		t.Fatalf("rpc_list must not appear as its own method")
	}
	if _, ok := byName["echo"]; !ok {
		t.Fatalf("expected custom echo, got %v", methodNames(created.Methods))
	}
	if !IsBuiltinRPCMethod("rpc-list") || !IsBuiltinRPCMethod("rpc_list") || IsBuiltinRPCMethod("echo") {
		t.Fatalf("IsBuiltinRPCMethod mismatch")
	}
}

func methodNames(methods []models.ProductMethod) []string {
	names := make([]string, 0, len(methods))
	for _, m := range methods {
		names = append(names, m.Name)
	}
	return names
}
