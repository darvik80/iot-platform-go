package api

import (
	"net/http"
	"testing"

	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
)

func TestUsersEndpointsRequirePlatformAdmin(t *testing.T) {
	e := newTestEnv(t)
	alice := e.user("alice")
	bob := e.user("bob")
	carol := e.user("carol")
	e.makePlatformAdmin(alice)

	// bob is admin of his personal account, but not of the platform.
	e.expect(e.do("GET", "/api/v1/users", "bob", nil), http.StatusForbidden, "bob lists users")
	e.expect(e.do("DELETE", "/api/v1/users/"+itoa(carol.ID), "bob", nil), http.StatusForbidden, "bob deletes carol")
	e.expect(e.do("PATCH", "/api/v1/users/"+itoa(carol.ID), "bob", map[string]any{"password": "hijacked123"}),
		http.StatusForbidden, "bob resets carol's password")
	e.expect(e.do("PATCH", "/api/v1/users/"+itoa(bob.ID), "bob", map[string]any{"role": "admin"}),
		http.StatusForbidden, "bob changes a global role")
	e.expect(e.do("PATCH", "/api/v1/users/"+itoa(bob.ID), "bob", map[string]any{"display_name": "Bobby"}),
		http.StatusOK, "bob edits himself")

	list := e.do("GET", "/api/v1/users", "alice", nil)
	e.expect(list, http.StatusOK, "platform admin lists users")
	if users := decode[[]models.User](t, list); len(users) != 3 {
		t.Errorf("platform admin sees %d users, want 3", len(users))
	}
	e.expect(e.do("PATCH", "/api/v1/users/"+itoa(carol.ID), "alice", map[string]any{"disabled": true}),
		http.StatusOK, "platform admin disables carol")
	e.expect(e.do("DELETE", "/api/v1/users/"+itoa(carol.ID), "alice", nil), http.StatusNoContent, "platform admin deletes carol")
}

func TestAccountAdminEditsOnlyExclusiveMembers(t *testing.T) {
	e := newTestEnv(t)
	e.user("bob")
	e.user("dave")
	bobAcc := e.accountID("bob")

	now := db.NowMS()
	res, err := e.plat.DB.Exec(`INSERT INTO users(email, password_hash, role, login, display_name, email_verified, created_at, updated_at)
		VALUES('member@example.com', '', 'viewer', 'member', 'Member', 1, ?, ?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	memberID, _ := res.LastInsertId()
	if _, err := e.plat.DB.Exec(`INSERT INTO account_memberships(user_id, account_id, role, status, joined_at) VALUES(?,?,?,?,?)`,
		memberID, bobAcc, "viewer", "active", now); err != nil {
		t.Fatal(err)
	}

	e.expect(e.do("PATCH", "/api/v1/users/"+itoa(memberID), "bob", map[string]any{"display_name": "Renamed"}),
		http.StatusOK, "bob edits his exclusive member")
	e.expect(e.do("PATCH", "/api/v1/users/"+itoa(memberID), "dave", map[string]any{"display_name": "Nope"}),
		http.StatusForbidden, "dave edits bob's member")

	// Once the member also belongs to dave's account, neither admin may take it over.
	if _, err := e.plat.DB.Exec(`INSERT INTO account_memberships(user_id, account_id, role, status, joined_at) VALUES(?,?,?,?,?)`,
		memberID, e.accountID("dave"), "viewer", "active", now); err != nil {
		t.Fatal(err)
	}
	e.expect(e.do("PATCH", "/api/v1/users/"+itoa(memberID), "bob", map[string]any{"password": "takeover123"}),
		http.StatusForbidden, "bob resets a shared member's password")
}

func TestUpdateUserValidatesEmail(t *testing.T) {
	e := newTestEnv(t)
	bob := e.user("bob")
	e.user("carol")

	e.expect(e.do("PATCH", "/api/v1/users/"+itoa(bob.ID), "bob", map[string]any{"email": "not-an-email"}),
		http.StatusBadRequest, "invalid email")
	e.expect(e.do("PATCH", "/api/v1/users/"+itoa(bob.ID), "bob", map[string]any{"email": "CAROL@example.com"}),
		http.StatusBadRequest, "email of another user")
	e.expect(e.do("PATCH", "/api/v1/users/"+itoa(bob.ID), "bob", map[string]any{"password": "short"}),
		http.StatusBadRequest, "short password")
	e.expect(e.do("PATCH", "/api/v1/users/"+itoa(bob.ID), "bob", map[string]any{"email": "Bob.New@Example.com"}),
		http.StatusOK, "valid new email")
	if u, _ := e.plat.GetUser(bob.ID); u.Email != "bob.new@example.com" {
		t.Errorf("email stored as %q, want lower-cased", u.Email)
	}
}
