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
	"golang.org/x/crypto/bcrypt"
)

func TestAdminCanCloneOrdinaryUserWithSharedPasswordAndPermissions(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "clone-users-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	admin, err := st.CreateInitialAdmin(ctx, "admin", "admin-hash")
	if err != nil {
		t.Fatal(err)
	}
	source, err := st.CreateUser(ctx, "teacher", "source-hash", store.User{
		CanEditStudents: true,
		CanAddStudents: false,
		CanResampleFaces: true,
		CanArrangeSeats: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: st}

	body := strings.NewReader(`{"usernames":["teacher01","teacher02","teacher03"],"password":"shared-pass-123"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/users/"+strconv.FormatInt(source.ID, 10)+"/clone", body)
	rec := httptest.NewRecorder()
	s.userAction(rec, withTestAdmin(req))
	if rec.Code != http.StatusCreated {
		t.Fatalf("clone status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Created int          `json:"created"`
		Users   []store.User `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Created != 3 || len(response.Users) != 3 {
		t.Fatalf("unexpected clone response: %#v", response)
	}

	hashes := make(map[string]struct{}, 3)
	for _, username := range []string{"teacher01", "teacher02", "teacher03"} {
		user, err := st.UserByUsername(ctx, username)
		if err != nil {
			t.Fatal(err)
		}
		if user.IsAdmin || !user.CanEditStudents || user.CanAddStudents || !user.CanResampleFaces || !user.CanArrangeSeats {
			t.Fatalf("cloned account %s has incorrect permissions: %#v", username, user)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("shared-pass-123")); err != nil {
			t.Fatalf("cloned account %s does not accept the shared password: %v", username, err)
		}
		if _, duplicate := hashes[user.PasswordHash]; duplicate {
			t.Fatalf("cloned accounts reused the same bcrypt hash for %s", username)
		}
		hashes[user.PasswordHash] = struct{}{}
	}

	adminCount, err := st.UserCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if adminCount != 5 {
		t.Fatalf("user count=%d, want admin + source + 3 clones", adminCount)
	}

	adminSourceBody := strings.NewReader(`{"usernames":["admin-copy"],"password":"shared-pass-123"}`)
	adminSourceReq := httptest.NewRequest(http.MethodPost, "/api/users/"+strconv.FormatInt(admin.ID, 10)+"/clone", adminSourceBody)
	adminSourceRec := httptest.NewRecorder()
	s.userAction(adminSourceRec, withTestAdmin(adminSourceReq))
	if adminSourceRec.Code == http.StatusCreated {
		t.Fatal("administrator account must not be cloneable")
	}
	if _, err := st.UserByUsername(ctx, "admin-copy"); err == nil {
		t.Fatal("administrator clone was unexpectedly created")
	}
}

func TestNonAdminCannotCloneUsersAndFailedBatchCreatesNothing(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "clone-users-security.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	source, err := st.CreateUser(ctx, "source", "source-hash", store.User{CanAddStudents: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser(ctx, "taken", "taken-hash", store.User{}); err != nil {
		t.Fatal(err)
	}
	s := &Server{store: st}

	unauthorizedBody := strings.NewReader(`{"usernames":["unauthorized01"],"password":"shared-pass-123"}`)
	unauthorizedReq := httptest.NewRequest(http.MethodPost, "/api/users/"+strconv.FormatInt(source.ID, 10)+"/clone", unauthorizedBody)
	unauthorizedReq = unauthorizedReq.WithContext(context.WithValue(unauthorizedReq.Context(), authContextKey{}, store.User{ID: 99}))
	unauthorizedRec := httptest.NewRecorder()
	s.userAction(unauthorizedRec, unauthorizedReq)
	if unauthorizedRec.Code != http.StatusForbidden {
		t.Fatalf("non-admin clone status=%d body=%s", unauthorizedRec.Code, unauthorizedRec.Body.String())
	}

	before, err := st.UserCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	duplicateBody := strings.NewReader(`{"usernames":["new01","TAKEN"],"password":"shared-pass-123"}`)
	duplicateReq := httptest.NewRequest(http.MethodPost, "/api/users/"+strconv.FormatInt(source.ID, 10)+"/clone", duplicateBody)
	duplicateRec := httptest.NewRecorder()
	s.userAction(duplicateRec, withTestAdmin(duplicateReq))
	if duplicateRec.Code != http.StatusBadRequest {
		t.Fatalf("duplicate batch status=%d body=%s", duplicateRec.Code, duplicateRec.Body.String())
	}
	after, err := st.UserCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("failed batch partially created users: before=%d after=%d", before, after)
	}
}
