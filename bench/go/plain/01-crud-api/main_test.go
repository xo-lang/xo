package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type client struct {
	t   *testing.T
	srv *httptest.Server
}

func newClient(t *testing.T) *client {
	t.Helper()
	srv := httptest.NewServer(NewHandler(NewStore()))
	t.Cleanup(srv.Close)
	return &client{t: t, srv: srv}
}

// do sends a request and decodes a JSON response into out when out is non-nil.
func (c *client) do(method, path, body string, out any) int {
	c.t.Helper()
	var rdr *bytes.Reader
	if body != "" {
		rdr = bytes.NewReader([]byte(body))
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, c.srv.URL+path, rdr)
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.srv.Client().Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			c.t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
	return resp.StatusCode
}

func strp(s string) *string { return &s }

func TestCreateThenGet(t *testing.T) {
	c := newClient(t)
	var created User
	if code := c.do("POST", "/users", `{"name":"Ada","email":"ada@example.com","nickname":"countess"}`, &created); code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", code)
	}
	if created.ID != 1 {
		t.Fatalf("first id = %d, want 1", created.ID)
	}
	var got User
	if code := c.do("GET", fmt.Sprintf("/users/%d", created.ID), "", &got); code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", code)
	}
	if got.ID != created.ID || got.Name != created.Name || got.Email != created.Email ||
		got.Nickname == nil || created.Nickname == nil || *got.Nickname != *created.Nickname {
		t.Fatalf("get = %+v, want %+v", got, created)
	}
	if got.Name != "Ada" || got.Email != "ada@example.com" || *got.Nickname != "countess" {
		t.Fatalf("unexpected fields: %+v", got)
	}
}

func TestDuplicateEmail(t *testing.T) {
	c := newClient(t)
	if code := c.do("POST", "/users", `{"name":"A","email":"x@y.z"}`, nil); code != http.StatusCreated {
		t.Fatalf("first create = %d", code)
	}
	var e map[string]string
	if code := c.do("POST", "/users", `{"name":"B","email":"x@y.z"}`, &e); code != http.StatusConflict {
		t.Fatalf("duplicate create = %d, want 409", code)
	}
	if e["error"] == "" {
		t.Fatalf("expected error message, got %v", e)
	}
	// Patching another user onto the taken email also conflicts.
	var second User
	if code := c.do("POST", "/users", `{"name":"B","email":"b@y.z"}`, &second); code != http.StatusCreated {
		t.Fatalf("second create = %d", code)
	}
	if code := c.do("PATCH", fmt.Sprintf("/users/%d", second.ID), `{"email":"x@y.z"}`, nil); code != http.StatusConflict {
		t.Fatalf("patch to duplicate email = %d, want 409", code)
	}
	// Patching a user to its own email is not a conflict.
	if code := c.do("PATCH", fmt.Sprintf("/users/%d", second.ID), `{"email":"b@y.z"}`, nil); code != http.StatusOK {
		t.Fatalf("patch to own email = %d, want 200", code)
	}
}

func TestInvalidInput(t *testing.T) {
	c := newClient(t)
	long := string(bytes.Repeat([]byte("a"), 101))
	cases := []struct{ name, body string }{
		{"email without at", `{"name":"A","email":"nope"}`},
		{"missing email", `{"name":"A"}`},
		{"missing name", `{"email":"a@b"}`},
		{"empty name", `{"name":"","email":"a@b"}`},
		{"long name", `{"name":"` + long + `","email":"a@b"}`},
		{"bad json", `{"name":`},
		{"wrong type", `{"name":1,"email":"a@b"}`},
		{"not object", `[]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var e map[string]string
			if code := c.do("POST", "/users", tc.body, &e); code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", code)
			}
			if e["error"] == "" {
				t.Fatalf("missing error message: %v", e)
			}
		})
	}
	// Exactly 100 characters (multi-byte) is accepted.
	name100 := string(bytes.Repeat([]byte("é"), 100))
	if code := c.do("POST", "/users", `{"name":"`+name100+`","email":"ok@b"}`, nil); code != http.StatusCreated {
		t.Fatalf("100 rune name = %d, want 201", code)
	}
	var u User
	c.do("POST", "/users", `{"name":"P","email":"p@b"}`, &u)
	if code := c.do("PATCH", fmt.Sprintf("/users/%d", u.ID), `{"email":"bad"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("patch invalid email = %d, want 400", code)
	}
}

func TestPatchNicknameOnly(t *testing.T) {
	c := newClient(t)
	var u User
	c.do("POST", "/users", `{"name":"Grace","email":"grace@navy.mil"}`, &u)
	var patched User
	if code := c.do("PATCH", fmt.Sprintf("/users/%d", u.ID), `{"nickname":"amazing"}`, &patched); code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200", code)
	}
	want := User{ID: u.ID, Name: "Grace", Email: "grace@navy.mil", Nickname: strp("amazing")}
	check := func(label string, got User) {
		t.Helper()
		if got.ID != want.ID || got.Name != want.Name || got.Email != want.Email ||
			got.Nickname == nil || *got.Nickname != *want.Nickname {
			t.Fatalf("%s = %+v, want %+v", label, got, want)
		}
	}
	check("patch response", patched)
	var got User
	c.do("GET", fmt.Sprintf("/users/%d", u.ID), "", &got)
	check("get after patch", got)

	if code := c.do("PATCH", "/users/999", `{"nickname":"x"}`, nil); code != http.StatusNotFound {
		t.Fatalf("patch missing = %d, want 404", code)
	}
}

func TestDeleteThenGet(t *testing.T) {
	c := newClient(t)
	var u User
	c.do("POST", "/users", `{"name":"Del","email":"del@x"}`, &u)
	if code := c.do("DELETE", fmt.Sprintf("/users/%d", u.ID), "", nil); code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", code)
	}
	if code := c.do("GET", fmt.Sprintf("/users/%d", u.ID), "", nil); code != http.StatusNotFound {
		t.Fatalf("get after delete = %d, want 404", code)
	}
	if code := c.do("DELETE", fmt.Sprintf("/users/%d", u.ID), "", nil); code != http.StatusNotFound {
		t.Fatalf("second delete = %d, want 404", code)
	}
	// Email is free again after delete.
	if code := c.do("POST", "/users", `{"name":"Del","email":"del@x"}`, nil); code != http.StatusCreated {
		t.Fatalf("recreate = %d, want 201", code)
	}
}

func TestListOrderedAndConcurrent(t *testing.T) {
	c := newClient(t)
	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := fmt.Sprintf(`{"name":"u%d","email":"u%d@x"}`, i, i)
			resp, err := c.srv.Client().Post(c.srv.URL+"/users", "application/json", bytes.NewReader([]byte(body)))
			if err != nil {
				errs <- err
				return
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusCreated {
				errs <- fmt.Errorf("status %d", resp.StatusCode)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var list []User
	if code := c.do("GET", "/users", "", &list); code != http.StatusOK {
		t.Fatalf("list = %d", code)
	}
	if len(list) != n {
		t.Fatalf("len = %d, want %d", len(list), n)
	}
	for i, u := range list {
		if u.ID != i+1 {
			t.Fatalf("list[%d].ID = %d, want %d", i, u.ID, i+1)
		}
	}
}

func TestEmptyListIsArray(t *testing.T) {
	c := newClient(t)
	var list []User
	if code := c.do("GET", "/users", "", &list); code != http.StatusOK {
		t.Fatalf("list = %d", code)
	}
	if list == nil || len(list) != 0 {
		t.Fatalf("want empty array, got %v", list)
	}
}

func TestAddrFromEnv(t *testing.T) {
	t.Setenv("PORT", "")
	if got := addr(); got != ":8080" {
		t.Fatalf("default addr = %q", got)
	}
	t.Setenv("PORT", "9191")
	if got := addr(); got != ":9191" {
		t.Fatalf("env addr = %q", got)
	}
}
