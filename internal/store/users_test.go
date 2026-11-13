package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestUserAccountsKeepAdministratorProtectedAndPersistPermissions(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	admin, err := st.CreateInitialAdmin(ctx, "admin", "admin-hash")
	if err != nil {
		t.Fatal(err)
	}
	if !admin.IsAdmin || !admin.CanEditStudents || !admin.CanAddStudents || !admin.CanResampleFaces || !admin.CanArrangeSeats {
		t.Fatalf("initial administrator permissions are incomplete: %#v", admin)
	}
	if _, err := st.CreateInitialAdmin(ctx, "second-admin", "hash"); err == nil {
		t.Fatal("second administrator bootstrap must be rejected")
	}

	created, err := st.CreateUser(ctx, "operator", "operator-hash", User{
		IsAdmin: true,
		CanEditStudents: true,
		CanAddStudents: false,
		CanResampleFaces: true,
		CanArrangeSeats: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.IsAdmin {
		t.Fatal("CreateUser must never create an administrator")
	}
	if !created.CanEditStudents || created.CanAddStudents || !created.CanResampleFaces || created.CanArrangeSeats {
		t.Fatalf("unexpected user permissions: %#v", created)
	}

	if err := st.UpdateUserAccess(ctx, admin.ID, User{}, ""); err == nil {
		t.Fatal("administrator permissions must be immutable")
	}
	if err := st.DeleteUser(ctx, admin.ID); err == nil {
		t.Fatal("administrator must not be deletable")
	}

	updated := User{CanEditStudents: false, CanAddStudents: true, CanResampleFaces: false, CanArrangeSeats: true}
	if err := st.UpdateUserAccess(ctx, created.ID, updated, ""); err != nil {
		t.Fatal(err)
	}
	users, err := st.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 || !users[0].IsAdmin {
		t.Fatalf("unexpected user list: %#v", users)
	}
	var listed User
	for _, user := range users {
		if user.ID == created.ID {
			listed = user
		}
	}
	if listed.CanEditStudents || !listed.CanAddStudents || listed.CanResampleFaces || !listed.CanArrangeSeats {
		t.Fatalf("permissions were not persisted: %#v", listed)
	}
	if listed.PasswordHash != "" {
		t.Fatal("user listing must not include password hashes")
	}
}

func TestUserSessionsExpireAndAreRemovedWithAccount(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "user-sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	user, err := st.CreateInitialAdmin(ctx, "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := st.CreateAuthSession(ctx, "session-hash", user.ID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UserBySession(ctx, "session-hash", now); err != nil {
		t.Fatalf("valid session was not found: %v", err)
	}
	if _, err := st.UserBySession(ctx, "session-hash", now.Add(2*time.Hour)); err == nil {
		t.Fatal("expired session must not authenticate")
	}
	other, err := st.CreateUser(ctx, "operator", "hash", User{})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAuthSession(ctx, "operator-session", other.ID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteUser(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UserBySession(ctx, "operator-session", now); err == nil {
		t.Fatal("deleting a user must invalidate its sessions")
	}
}
