package api

import (
	"testing"

	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
)

func (e *testEnv) listProductIDs(as string) []string {
	e.t.Helper()
	rec := e.do("GET", "/api/v1/products", as, nil)
	e.expect(rec, 200, "list products")
	var ids []string
	for _, p := range decode[[]models.Product](e.t, rec) {
		ids = append(ids, p.ProductID)
	}
	return ids
}

func TestCreatedProductIsListed(t *testing.T) {
	e := newTestEnv(t)
	e.user("alice")

	rec := e.do("POST", "/api/v1/products", "alice", map[string]string{"product_id": "esp32-cam", "name": "Cam"})
	e.expect(rec, 201, "create product")

	if ids := e.listProductIDs("alice"); len(ids) != 1 || ids[0] != "esp32-cam" {
		t.Fatalf("list = %v, want [esp32-cam]", ids)
	}
}

func TestProductFollowsActiveAccount(t *testing.T) {
	e := newTestEnv(t)
	alice := e.user("alice")
	e.user("demo")
	demoAcc := e.accountID("demo")
	if _, err := e.plat.DB.Exec(`INSERT INTO account_memberships(user_id, account_id, role, status, joined_at) VALUES(?,?,?,?,?)`,
		alice.ID, demoAcc, models.RoleAdmin, "active", db.NowMS()); err != nil {
		t.Fatal(err)
	}

	rec := e.do("POST", "/api/v1/auth/switch-account", "alice", map[string]int64{"account_id": demoAcc})
	e.expect(rec, 200, "switch account")
	e.tokens["alice@demo"] = decode[map[string]string](t, rec)["token"]

	e.expect(e.do("POST", "/api/v1/products", "alice@demo", map[string]string{"product_id": "main"}), 201, "create in demo")

	if ids := e.listProductIDs("alice@demo"); len(ids) != 1 || ids[0] != "main" {
		t.Errorf("demo list = %v, want [main]", ids)
	}
	if ids := e.listProductIDs("demo"); len(ids) != 1 || ids[0] != "main" {
		t.Errorf("demo owner list = %v, want [main]", ids)
	}
	if ids := e.listProductIDs("alice"); len(ids) != 0 {
		t.Errorf("alice personal list = %v, want empty", ids)
	}
}
