package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type response struct {
	status int
	body   []byte
}

func do(t *testing.T, h http.Handler, method, path, body string) response {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return response{status: rec.Code, body: rec.Body.Bytes()}
}

func decodeUser(t *testing.T, r response) User {
	t.Helper()
	var u User
	if err := json.Unmarshal(r.body, &u); err != nil {
		t.Fatalf("decode user from %q: %v", r.body, err)
	}
	return u
}

func expectStatus(t *testing.T, r response, want int) {
	t.Helper()
	if r.status != want {
		t.Fatalf("status = %d, want %d (body %q)", r.status, want, r.body)
	}
}

func expectErrorBody(t *testing.T, r response) {
	t.Helper()
	var e errorBody
	if err := json.Unmarshal(r.body, &e); err != nil {
		t.Fatalf("decode error body %q: %v", r.body, err)
	}
	if e.Error == "" {
		t.Fatalf("error body has empty error field: %q", r.body)
	}
}

func nick(u User) string {
	if u.Nickname == nil {
		return "<nil>"
	}
	return *u.Nickname
}

func TestCreateThenGet(t *testing.T) {
	h := NewHandler(NewStore())
	r := do(t, h, http.MethodPost, "/users", `{"name":"Ada","email":"ada@example.com","nickname":"countess"}`)
	expectStatus(t, r, http.StatusCreated)
	created := decodeUser(t, r)
	if created.ID != 1 {
		t.Fatalf("first id = %d, want 1", created.ID)
	}
	if created.Name != "Ada" || created.Email != "ada@example.com" || nick(created) != "countess" {
		t.Fatalf("unexpected created user %+v", created)
	}

	r = do(t, h, http.MethodGet, "/users/1", "")
	expectStatus(t, r, http.StatusOK)
	got := decodeUser(t, r)
	if got.ID != created.ID || got.Name != created.Name || got.Email != created.Email || nick(got) != nick(created) {
		t.Fatalf("get = %+v, want %+v", got, created)
	}
}

func TestDuplicateEmailReturns409(t *testing.T) {
	h := NewHandler(NewStore())
	expectStatus(t, do(t, h, http.MethodPost, "/users", `{"name":"A","email":"a@x.com"}`), http.StatusCreated)
	r := do(t, h, http.MethodPost, "/users", `{"name":"B","email":"a@x.com"}`)
	expectStatus(t, r, http.StatusConflict)
	expectErrorBody(t, r)

	// Patch into an existing email also conflicts.
	expectStatus(t, do(t, h, http.MethodPost, "/users", `{"name":"C","email":"c@x.com"}`), http.StatusCreated)
	r = do(t, h, http.MethodPatch, "/users/2", `{"email":"a@x.com"}`)
	expectStatus(t, r, http.StatusConflict)
	expectErrorBody(t, r)

	// Patching a user to its own email is fine.
	expectStatus(t, do(t, h, http.MethodPatch, "/users/1", `{"email":"a@x.com"}`), http.StatusOK)
}

func TestInvalidInputReturns400(t *testing.T) {
	h := NewHandler(NewStore())
	cases := map[string]string{
		"email without at":  `{"name":"A","email":"no-at-sign"}`,
		"missing email":     `{"name":"A"}`,
		"missing name":      `{"email":"a@x.com"}`,
		"empty name":        `{"name":"","email":"a@x.com"}`,
		"long name":         `{"name":"` + strings.Repeat("n", 101) + `","email":"a@x.com"}`,
		"malformed json":    `{"name":`,
		"wrong type":        `{"name":5,"email":"a@x.com"}`,
		"unknown field":     `{"name":"A","email":"a@x.com","age":3}`,
		"trailing garbage":  `{"name":"A","email":"a@x.com"} {}`,
		"null email":        `{"name":"A","email":null}`,
		"nickname not text": `{"name":"A","email":"a@x.com","nickname":1}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			r := do(t, h, http.MethodPost, "/users", body)
			expectStatus(t, r, http.StatusBadRequest)
			expectErrorBody(t, r)
		})
	}

	// 100 multi byte characters is valid.
	expectStatus(t, do(t, h, http.MethodPost, "/users", `{"name":"`+strings.Repeat("é", 100)+`","email":"e@x.com"}`), http.StatusCreated)

	patchCases := map[string]string{
		"invalid email": `{"email":"bad"}`,
		"empty name":    `{"name":""}`,
		"null name":     `{"name":null}`,
		"not object":    `[1,2]`,
		"unknown field": `{"id":7}`,
	}
	for name, body := range patchCases {
		t.Run("patch "+name, func(t *testing.T) {
			r := do(t, h, http.MethodPatch, "/users/1", body)
			expectStatus(t, r, http.StatusBadRequest)
			expectErrorBody(t, r)
		})
	}
}

func TestPatchNicknameKeepsOtherFields(t *testing.T) {
	h := NewHandler(NewStore())
	expectStatus(t, do(t, h, http.MethodPost, "/users", `{"name":"Grace","email":"grace@navy.mil"}`), http.StatusCreated)

	r := do(t, h, http.MethodPatch, "/users/1", `{"nickname":"amazing"}`)
	expectStatus(t, r, http.StatusOK)
	u := decodeUser(t, r)
	if u.ID != 1 || u.Name != "Grace" || u.Email != "grace@navy.mil" || nick(u) != "amazing" {
		t.Fatalf("patched user = %+v (nickname %s)", u, nick(u))
	}

	got := decodeUser(t, do(t, h, http.MethodGet, "/users/1", ""))
	if got.Name != "Grace" || got.Email != "grace@navy.mil" || nick(got) != "amazing" {
		t.Fatalf("stored user = %+v (nickname %s)", got, nick(got))
	}

	// Null clears the nickname.
	cleared := decodeUser(t, do(t, h, http.MethodPatch, "/users/1", `{"nickname":null}`))
	if cleared.Nickname != nil || cleared.Name != "Grace" {
		t.Fatalf("cleared user = %+v", cleared)
	}

	// Patch of a missing user is 404.
	expectStatus(t, do(t, h, http.MethodPatch, "/users/99", `{"nickname":"x"}`), http.StatusNotFound)
}

func TestDeleteThenGetReturns404(t *testing.T) {
	h := NewHandler(NewStore())
	expectStatus(t, do(t, h, http.MethodPost, "/users", `{"name":"A","email":"a@x.com"}`), http.StatusCreated)
	r := do(t, h, http.MethodDelete, "/users/1", "")
	expectStatus(t, r, http.StatusNoContent)
	if len(r.body) != 0 {
		t.Fatalf("204 body should be empty, got %q", r.body)
	}
	r = do(t, h, http.MethodGet, "/users/1", "")
	expectStatus(t, r, http.StatusNotFound)
	expectErrorBody(t, r)
	expectStatus(t, do(t, h, http.MethodDelete, "/users/1", ""), http.StatusNotFound)

	// The email is free again after delete, and ids are not reused.
	r = do(t, h, http.MethodPost, "/users", `{"name":"A2","email":"a@x.com"}`)
	expectStatus(t, r, http.StatusCreated)
	if u := decodeUser(t, r); u.ID != 2 {
		t.Fatalf("id after delete = %d, want 2", u.ID)
	}
}

func TestListOrderedByID(t *testing.T) {
	h := NewHandler(NewStore())
	r := do(t, h, http.MethodGet, "/users", "")
	expectStatus(t, r, http.StatusOK)
	if strings.TrimSpace(string(r.body)) != "[]" {
		t.Fatalf("empty list body = %q, want []", r.body)
	}
	for i := 1; i <= 5; i++ {
		body := fmt.Sprintf(`{"name":"u%d","email":"u%d@x.com"}`, i, i)
		expectStatus(t, do(t, h, http.MethodPost, "/users", body), http.StatusCreated)
	}
	r = do(t, h, http.MethodGet, "/users", "")
	expectStatus(t, r, http.StatusOK)
	var users []User
	if err := json.Unmarshal(r.body, &users); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(users) != 5 {
		t.Fatalf("len = %d, want 5", len(users))
	}
	for i, u := range users {
		if u.ID != int64(i+1) {
			t.Fatalf("users[%d].ID = %d, want %d", i, u.ID, i+1)
		}
	}
}

func TestBadIDReturns404(t *testing.T) {
	h := NewHandler(NewStore())
	for _, path := range []string{"/users/abc", "/users/0", "/users/-1"} {
		expectStatus(t, do(t, h, http.MethodGet, path, ""), http.StatusNotFound)
	}
}

func TestConcurrentCreates(t *testing.T) {
	h := NewHandler(NewStore())
	const n = 50
	var wg sync.WaitGroup
	ids := make(chan int64, n)
	errs := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/users",
				strings.NewReader(fmt.Sprintf(`{"name":"u","email":"u%d@x.com"}`, i)))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusCreated {
				errs <- fmt.Sprintf("status %d", rec.Code)
				return
			}
			var u User
			if err := json.Unmarshal(rec.Body.Bytes(), &u); err != nil {
				errs <- err.Error()
				return
			}
			ids <- u.ID
			// Exercise concurrent reads too.
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/users", nil))
		}(i)
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		t.Errorf("concurrent create: %s", e)
	}
	seen := make(map[int64]bool)
	for id := range ids {
		if seen[id] || id < 1 || id > n {
			t.Fatalf("bad or duplicate id %d", id)
		}
		seen[id] = true
	}
	if len(seen) != n {
		t.Fatalf("got %d ids, want %d", len(seen), n)
	}
}

func TestListenAddr(t *testing.T) {
	t.Setenv("PORT", "")
	if got := listenAddr(); got != ":8080" {
		t.Fatalf("default addr = %q", got)
	}
	t.Setenv("PORT", "9999")
	if got := listenAddr(); got != ":9999" {
		t.Fatalf("addr = %q", got)
	}
}
