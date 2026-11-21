package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestPrimaryAdminCanGrantAndRevokeDelegatedAdmin(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "admin-delegation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	primary, err := st.CreateInitialAdmin(ctx, "primary", "primary-hash")
	if err != nil {
		t.Fatal(err)
	}
	if !primary.IsAdmin || !primary.IsPrimaryAdmin {
		t.Fatalf("bootstrap account must be the primary administrator: %#v", primary)
	}

	delegated, err := st.CreateUser(ctx, "delegated", "delegated-hash", User{
		CanEditSettings: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := st.CreateUser(ctx, "ordinary", "ordinary-hash", User{
		CanAddStudents: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := st.CreateAuthSession(ctx, "delegated-before-grant", delegated.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	granted, err := st.SetUserAdminRole(ctx, primary.ID, delegated.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !granted.IsAdmin || granted.IsPrimaryAdmin {
		t.Fatalf("delegated administrator role was not assigned correctly: %#v", granted)
	}
	if !granted.CanEditSettings {
		t.Fatal("granting administrator status must preserve the user's existing granular permissions")
	}
	byName, err := st.UserByUsername(ctx, "delegated")
	if err != nil {
		t.Fatal(err)
	}
	if !byName.IsAdmin || byName.IsPrimaryAdmin {
		t.Fatalf("username lookup did not return the delegated administrator role: %#v", byName)
	}
	if _, err := st.UserBySession(ctx, "delegated-before-grant", time.Now()); err == nil {
		t.Fatal("granting administrator status must expire existing sessions so the UI reloads")
	}

	if _, err := st.SetUserAdminRole(ctx, delegated.ID, ordinary.ID, true); err == nil {
		t.Fatal("a delegated administrator must not grant administrator status to another account")
	}
	if _, err := st.SetUserAdminRole(ctx, primary.ID, primary.ID, false); err == nil {
		t.Fatal("the primary administrator must not be able to revoke its own protected role")
	}

	if err := st.CreateAuthSession(ctx, "delegated-after-grant", delegated.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	active, err := st.UserBySession(ctx, "delegated-after-grant", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !active.IsAdmin || active.IsPrimaryAdmin {
		t.Fatalf("new session did not load delegated administrator role: %#v", active)
	}

	revoked, err := st.SetUserAdminRole(ctx, primary.ID, delegated.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.IsAdmin || revoked.IsPrimaryAdmin || !revoked.CanEditSettings {
		t.Fatalf("revocation must restore the account's original granular permissions: %#v", revoked)
	}
	if _, err := st.UserBySession(ctx, "delegated-after-grant", time.Now()); err == nil {
		t.Fatal("revoking administrator status must expire the target's active sessions")
	}
	primaryAfter, err := st.UserByUsername(ctx, "primary")
	if err != nil {
		t.Fatal(err)
	}
	if !primaryAfter.IsAdmin || !primaryAfter.IsPrimaryAdmin {
		t.Fatalf("primary administrator identity changed unexpectedly: %#v", primaryAfter)
	}
}

func TestLegacyUsersDatabaseMarksOldestAdminAsPrimary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-admins.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			is_admin INTEGER NOT NULL DEFAULT 0,
			can_edit_students INTEGER NOT NULL DEFAULT 0,
			can_add_students INTEGER NOT NULL DEFAULT 0,
			can_resample_faces INTEGER NOT NULL DEFAULT 0,
			can_arrange_seats INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);
		INSERT INTO users(username,password_hash,is_admin,created_at,updated_at)
		VALUES('first-admin','hash-1',1,1,1);
		INSERT INTO users(username,password_hash,is_admin,created_at,updated_at)
		VALUES('second-admin','hash-2',1,2,2);
	`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	users, err := st.ListUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 {
		t.Fatalf("legacy users count=%d, want 2", len(users))
	}
	if users[0].Username != "first-admin" || !users[0].IsPrimaryAdmin || !users[0].IsAdmin {
		t.Fatalf("oldest existing administrator must become the primary administrator: %#v", users[0])
	}
	if users[1].IsPrimaryAdmin {
		t.Fatalf("legacy secondary administrator must not become primary: %#v", users[1])
	}
}
