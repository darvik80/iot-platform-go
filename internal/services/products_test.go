package services

import (
	"path/filepath"
	"testing"

	"crearts-iot-platform/internal/config"
	"crearts-iot-platform/internal/db"
	"crearts-iot-platform/internal/models"
)

func TestUpdateProductMethods(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	p := New(database, config.Config{}, nil, nil)

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
}

func methodNames(methods []models.ProductMethod) []string {
	names := make([]string, 0, len(methods))
	for _, m := range methods {
		names = append(names, m.Name)
	}
	return names
}
