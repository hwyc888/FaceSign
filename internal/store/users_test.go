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


func TestPasswordResetExpiresSessionsButKeepsCameraReleaseToken(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "user-reset-sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "operator", "old-hash", User{})
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour)
	if err := st.CreateAuthSession(ctx, "operator-token", user.ID, expires); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateUserAccess(ctx, user.ID, User{}, "new-hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UserBySession(ctx, "operator-token", time.Now()); err == nil {
		t.Fatal("password reset must revoke normal API access")
	}
	if _, err := st.UserBySessionForRelease(ctx, "operator-token"); err != nil {
		t.Fatalf("revoked session should remain available for camera release: %v", err)
	}
}


func TestCloneUsersCopiesOnlyOrdinaryPermissionsWithSharedPasswordHashes(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "clone-users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	admin, err := st.CreateInitialAdmin(ctx, "admin", "admin-hash")
	if err != nil {
		t.Fatal(err)
	}
	source, err := st.CreateUser(ctx, "source", "source-hash", User{
		CanEditStudents: true,
		CanAddStudents: false,
		CanResampleFaces: true,
		CanArrangeSeats: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	created, err := st.CloneUsers(ctx, source.ID, []UserCloneCredential{
		{Username: "copy01", PasswordHash: "bcrypt-hash-01"},
		{Username: "copy02", PasswordHash: "bcrypt-hash-02"},
		{Username: "copy03", PasswordHash: "bcrypt-hash-03"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 3 {
		t.Fatalf("created %d accounts, want 3", len(created))
	}
	for _, username := range []string{"copy01", "copy02", "copy03"} {
		user, err := st.UserByUsername(ctx, username)
		if err != nil {
			t.Fatal(err)
		}
		if user.IsAdmin {
			t.Fatalf("%s was unexpectedly created as administrator", username)
		}
		if !user.CanEditStudents || user.CanAddStudents || !user.CanResampleFaces || user.CanArrangeSeats {
			t.Fatalf("%s did not inherit source permissions: %#v", username, user)
		}
		if user.PasswordHash == "" || user.PasswordHash == "source-hash" {
			t.Fatalf("%s did not receive a new password hash", username)
		}
	}
	if _, err := st.CloneUsers(ctx, admin.ID, []UserCloneCredential{
		{Username: "admin-copy", PasswordHash: "hash"},
	}); err == nil {
		t.Fatal("administrator account must not be a clone source")
	}
	if _, err := st.UserByUsername(ctx, "admin-copy"); err == nil {
		t.Fatal("administrator clone was unexpectedly created")
	}
}

func TestCloneUsersBatchIsAtomicWhenUsernameAlreadyExists(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "clone-users-atomic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	source, err := st.CreateUser(ctx, "source", "source-hash", User{CanAddStudents: true})
	if err != nil {
		t.Fatal(err)
	}
	existing, err := st.CreateUser(ctx, "taken", "taken-hash", User{})
	if err != nil {
		t.Fatal(err)
	}
	before, err := st.UserCount(ctx)
	if err != nil {
		t.Fatal(err)
	}

	_, err = st.CloneUsers(ctx, source.ID, []UserCloneCredential{
		{Username: "new01", PasswordHash: "hash-01"},
		{Username: "TAKEN", PasswordHash: "hash-02"},
		{Username: "new03", PasswordHash: "hash-03"},
	})
	if err == nil {
		t.Fatal("expected case-insensitive username collision")
	}
	after, err := st.UserCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("failed clone batch left partial accounts: count before=%d after=%d", before, after)
	}
	if _, err := st.UserByUsername(ctx, "new01"); err == nil {
		t.Fatal("first account from failed batch should have been rolled back")
	}
	if _, err := st.UserByUsername(ctx, "new03"); err == nil {
		t.Fatal("last account from failed batch should not have been created")
	}
	if existing.Username != "taken" {
		t.Fatalf("existing account unexpectedly changed: %#v", existing)
	}
}
