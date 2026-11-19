package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestCanEditSettingsPermissionPersistsAcrossSessionsAndClones(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "settings-permission.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	admin, err := st.CreateInitialAdmin(ctx, "admin", "admin-hash")
	if err != nil {
		t.Fatal(err)
	}
	if !admin.CanEditSettings {
		t.Fatal("administrator must have settings permission")
	}

	operator, err := st.CreateUser(ctx, "operator", "operator-hash", User{CanEditSettings: true})
	if err != nil {
		t.Fatal(err)
	}
	byName, err := st.UserByUsername(ctx, "operator")
	if err != nil || !byName.CanEditSettings {
		t.Fatalf("settings permission was not persisted in username lookup: user=%#v err=%v", byName, err)
	}

	if err := st.CreateAuthSession(ctx, "settings-session", operator.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	bySession, err := st.UserBySession(ctx, "settings-session", time.Now())
	if err != nil || !bySession.CanEditSettings {
		t.Fatalf("settings permission was not loaded into authenticated session: user=%#v err=%v", bySession, err)
	}

	listed, err := st.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, user := range listed {
		if user.ID == operator.ID {
			found = user.CanEditSettings
		}
	}
	if !found {
		t.Fatal("settings permission was not returned by ListUsers")
	}

	cloned, err := st.CloneUsers(ctx, operator.ID, []UserCloneCredential{
		{Username: "operator-copy", PasswordHash: "copy-hash"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cloned) != 1 || !cloned[0].CanEditSettings {
		t.Fatalf("cloned account did not inherit settings permission: %#v", cloned)
	}

	if err := st.UpdateUserAccess(ctx, operator.ID, User{}, ""); err != nil {
		t.Fatal(err)
	}
	updated, err := st.UserByUsername(ctx, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if updated.CanEditSettings {
		t.Fatal("settings permission could not be revoked")
	}
}
