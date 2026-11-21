package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hwyc888/FaceSign/internal/store"
)

func requestAsAdminRoleUser(req *http.Request, user store.User) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), authContextKey{}, user))
}

func TestOnlyPrimaryAdminCanGrantAndRevokeAdministratorRole(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "admin-role-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	primary, err := st.CreateInitialAdmin(ctx, "primary", "primary-hash")
	if err != nil {
		t.Fatal(err)
	}
	delegated, err := st.CreateUser(ctx, "delegated", "delegated-hash", store.User{CanEditSettings: true})
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := st.CreateUser(ctx, "ordinary", "ordinary-hash", store.User{CanAddStudents: true})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: st}

	grantReq := httptest.NewRequest(http.MethodPost, "/api/users/"+strconv.FormatInt(delegated.ID, 10)+"/admin", nil)
	grantRec := httptest.NewRecorder()
	s.userAction(grantRec, requestAsAdminRoleUser(grantReq, primary))
	if grantRec.Code != http.StatusOK {
		t.Fatalf("primary admin grant status=%d body=%s", grantRec.Code, grantRec.Body.String())
	}
	var grantResponse struct {
		OK   bool       `json:"ok"`
		User store.User `json:"user"`
	}
	if err := json.Unmarshal(grantRec.Body.Bytes(), &grantResponse); err != nil {
		t.Fatal(err)
	}
	if !grantResponse.OK || !grantResponse.User.IsAdmin || grantResponse.User.IsPrimaryAdmin {
		t.Fatalf("unexpected grant response: %#v", grantResponse)
	}

	delegatedUser, err := st.UserByUsername(ctx, "delegated")
	if err != nil {
		t.Fatal(err)
	}
	unauthorizedReq := httptest.NewRequest(http.MethodPost, "/api/users/"+strconv.FormatInt(ordinary.ID, 10)+"/admin", nil)
	unauthorizedRec := httptest.NewRecorder()
	s.userAction(unauthorizedRec, requestAsAdminRoleUser(unauthorizedReq, delegatedUser))
	if unauthorizedRec.Code != http.StatusForbidden {
		t.Fatalf("delegated admin must not promote another account: status=%d body=%s", unauthorizedRec.Code, unauthorizedRec.Body.String())
	}

	ordinaryReq := httptest.NewRequest(http.MethodPost, "/api/users/"+strconv.FormatInt(ordinary.ID, 10)+"/admin", nil)
	ordinaryRec := httptest.NewRecorder()
	s.userAction(ordinaryRec, requestAsAdminRoleUser(ordinaryReq, ordinary))
	if ordinaryRec.Code != http.StatusForbidden {
		t.Fatalf("ordinary user must not promote accounts: status=%d body=%s", ordinaryRec.Code, ordinaryRec.Body.String())
	}

	revokeReq := httptest.NewRequest(http.MethodDelete, "/api/users/"+strconv.FormatInt(delegated.ID, 10)+"/admin", nil)
	revokeRec := httptest.NewRecorder()
	s.userAction(revokeRec, requestAsAdminRoleUser(revokeReq, primary))
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("primary admin revoke status=%d body=%s", revokeRec.Code, revokeRec.Body.String())
	}
	revoked, err := st.UserByUsername(ctx, "delegated")
	if err != nil {
		t.Fatal(err)
	}
	if revoked.IsAdmin || revoked.IsPrimaryAdmin || !revoked.CanEditSettings {
		t.Fatalf("revoked user should return to the original granular permissions: %#v", revoked)
	}

	selfReq := httptest.NewRequest(http.MethodDelete, "/api/users/"+strconv.FormatInt(primary.ID, 10)+"/admin", nil)
	selfRec := httptest.NewRecorder()
	s.userAction(selfRec, requestAsAdminRoleUser(selfReq, primary))
	if selfRec.Code == http.StatusOK {
		t.Fatal("primary administrator must not be able to revoke itself")
	}
	primaryAfter, err := st.UserByUsername(ctx, "primary")
	if err != nil {
		t.Fatal(err)
	}
	if !primaryAfter.IsAdmin || !primaryAfter.IsPrimaryAdmin {
		t.Fatalf("primary administrator was changed: %#v", primaryAfter)
	}
}

func TestDelegatedAdminReceivesAllAdminSettingsPermissions(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "delegated-admin-settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	primary, err := st.CreateInitialAdmin(ctx, "primary", "primary-hash")
	if err != nil {
		t.Fatal(err)
	}
	delegated, err := st.CreateUser(ctx, "delegated", "delegated-hash", store.User{})
	if err != nil {
		t.Fatal(err)
	}
	delegated, err = st.SetUserAdminRole(ctx, primary.ID, delegated.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: st}

	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"disable_liveness_detection":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.settings(rec, requestAsAdminRoleUser(req, delegated))
	if rec.Code != http.StatusOK {
		t.Fatalf("delegated admin should have full administrator settings access: status=%d body=%s", rec.Code, rec.Body.String())
	}
	settings, err := st.AppSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.DisableLivenessDetection {
		t.Fatal("delegated admin's full settings permission did not persist")
	}
}
