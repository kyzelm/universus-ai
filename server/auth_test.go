package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// fakeStore is the second implementation the store interface exists for.
type fakeStore struct {
	mu     sync.Mutex
	byID   map[int64]User
	hashes map[string]string // email -> password hash
	lp     map[int64]int     // ladder points, which only the queue reads
	next   int64
	fail   error // forced, for the "the database is on fire" paths
}

func newStore() *fakeStore {
	return &fakeStore{byID: map[int64]User{}, hashes: map[string]string{}, lp: map[int64]int{}, next: 1}
}

func (f *fakeStore) createUser(_ context.Context, email, hash, name string) (User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return User{}, f.fail
	}
	for _, u := range f.byID {
		if u.Email == email || u.DisplayName == name {
			return User{}, errTaken
		}
	}
	u := User{ID: f.next, Email: email, DisplayName: name}
	f.next++
	f.byID[u.ID] = u
	f.hashes[email] = hash
	return u, nil
}

func (f *fakeStore) credentials(_ context.Context, email string) (User, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return User{}, "", f.fail
	}
	for _, u := range f.byID {
		if u.Email == email {
			return u, f.hashes[email], nil
		}
	}
	return User{}, "", errNoUser
}

func (f *fakeStore) ratingOf(_ context.Context, id int64) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lp[id]
}

func (f *fakeStore) rating(_ context.Context, id int64) Rating {
	f.mu.Lock()
	defer f.mu.Unlock()
	lp := f.lp[id]
	return Rating{LP: lp, Tier: tierOf(lp), TierName: tierNames[tierOf(lp)]}
}

func (f *fakeStore) userByID(_ context.Context, id int64) (User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return User{}, errNoUser
}

func testAuth() (*auth, *http.ServeMux) {
	a := &auth{users: newStore(), secret: []byte("test-secret")}
	mux := http.NewServeMux()
	a.routes(mux)
	return a, mux
}

func post(t *testing.T, mux *http.ServeMux, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func decodeSession(t *testing.T, w *httptest.ResponseRecorder) session {
	t.Helper()
	var s session
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatalf("decoding %q: %v", w.Body.String(), err)
	}
	return s
}

const goodRegister = `{"email":"Player@Example.COM ","password":"correct horse","displayName":"player-one"}`

func TestRegisterThenLogIn(t *testing.T) {
	_, mux := testAuth()

	w := post(t, mux, "/api/auth/register", goodRegister)
	if w.Code != http.StatusOK {
		t.Fatalf("register: %d %s", w.Code, w.Body)
	}
	got := decodeSession(t, w)
	// Trimmed and lowercased on the way in, or the unique index does not mean what everybody
	// assumes it means.
	if got.User.Email != "player@example.com" {
		t.Errorf("stored email %q, want it normalized", got.User.Email)
	}
	if got.Token == "" {
		t.Error("registered with no token: the client would have to log in again immediately")
	}

	w = post(t, mux, "/api/auth/login",
		`{"email":"player@example.com","password":"correct horse"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	if decodeSession(t, w).User.ID != got.User.ID {
		t.Error("logged in as somebody else")
	}
}

// A password is never echoed and a hash never leaves the server.
func TestNoSecretsInAnyResponse(t *testing.T) {
	_, mux := testAuth()

	for _, w := range []*httptest.ResponseRecorder{
		post(t, mux, "/api/auth/register", goodRegister),
		post(t, mux, "/api/auth/login", `{"email":"player@example.com","password":"correct horse"}`),
	} {
		body := w.Body.String()
		for _, secret := range []string{"correct horse", "$2a$", "$2b$", "password_hash"} {
			if strings.Contains(body, secret) {
				t.Errorf("a response contained %q: %s", secret, body)
			}
		}
	}
}

func TestProfileNeedsARealToken(t *testing.T) {
	a, mux := testAuth()
	token := decodeSession(t, post(t, mux, "/api/auth/register", goodRegister)).Token

	get := func(header string) int {
		req := httptest.NewRequest("GET", "/api/profile", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w.Code
	}

	if code := get("Bearer " + token); code != http.StatusOK {
		t.Errorf("a real token got %d", code)
	}
	for _, header := range []string{
		"",
		token,                   // no Bearer prefix
		"Bearer " + token + "x", // signature no longer matches
		"Bearer not.a.token",
	} {
		if code := get(header); code != http.StatusUnauthorized {
			t.Errorf("%q got %d, want 401", header, code)
		}
	}

	// A token signed with a different key is the attack this is all for.
	other := &auth{users: a.users, secret: []byte("a-different-secret")}
	forged, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "1",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString(other.secret)
	if err != nil {
		t.Fatal(err)
	}
	if code := get("Bearer " + forged); code != http.StatusUnauthorized {
		t.Errorf("a token signed with another key got %d", code)
	}
}

func TestAnExpiredTokenIsRefused(t *testing.T) {
	a, _ := testAuth()

	stale, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "1",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
	}).SignedString(a.secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.userID("Bearer " + stale); err == nil {
		t.Error("an expired token was accepted")
	}

	// And one with no expiry at all, which is a token that never stops working.
	forever, err := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.RegisteredClaims{Subject: "1"}).SignedString(a.secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.userID("Bearer " + forever); err == nil {
		t.Error("a token with no expiry was accepted")
	}
}

// The hash actually protects the password.
func TestThePasswordIsStoredAsABcryptHash(t *testing.T) {
	a, mux := testAuth()
	post(t, mux, "/api/auth/register", goodRegister)

	_, hash, err := a.users.credentials(context.Background(), "player@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse" || !strings.HasPrefix(hash, "$2") {
		t.Fatalf("stored %q, want a bcrypt hash", hash)
	}
	if cost, err := bcrypt.Cost([]byte(hash)); err != nil || cost != bcryptCost {
		t.Errorf("cost %d (err %v), want %d", cost, err, bcryptCost)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("correct horse")); err != nil {
		t.Errorf("the stored hash does not verify against the password: %v", err)
	}
}
