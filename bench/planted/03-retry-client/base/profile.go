package benchtask

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Profile is a user profile returned by the profile service.
type Profile struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Fetcher bundles the dependencies needed by FetchProfile.
type Fetcher struct {
	Net     Doer
	Clock   Clock
	Rand    Rand
	Policy  RetryPolicy
	BaseURL string // e.g. "https://api.example.com"
}

// ProfileErrKind classifies a ProfileError.
type ProfileErrKind int

// ProfileError kinds.
const (
	ProfileKindFetch ProfileErrKind = iota + 1
	ProfileKindDecode
)

func (k ProfileErrKind) String() string {
	switch k {
	case ProfileKindFetch:
		return "fetch"
	case ProfileKindDecode:
		return "decode"
	default:
		return "unknown"
	}
}

// ProfileError is returned by FetchProfile. For ProfileKindFetch, Err is a
// *FetchError; for ProfileKindDecode, Err is the JSON decoding error.
type ProfileError struct {
	Kind   ProfileErrKind
	UserID int
	Err    error
}

func (e *ProfileError) Error() string {
	return fmt.Sprintf("profile %d: %s: %v", e.UserID, e.Kind, e.Err)
}

// Unwrap returns the underlying error.
func (e *ProfileError) Unwrap() error { return e.Err }

// FetchProfile fetches and decodes the profile for userID from
// BaseURL + "/users/{id}".
func (f *Fetcher) FetchProfile(ctx context.Context, userID int) (Profile, error) {
	u := strings.TrimRight(f.BaseURL, "/") + "/users/" + strconv.Itoa(userID)
	body, err := GetWithRetry(ctx, f.Net, f.Clock, f.Rand, u, f.Policy)
	if err != nil {
		return Profile{}, &ProfileError{Kind: ProfileKindFetch, UserID: userID, Err: err}
	}
	var p Profile
	if err := json.Unmarshal(body, &p); err != nil {
		return Profile{}, &ProfileError{Kind: ProfileKindDecode, UserID: userID, Err: err}
	}
	return p, nil
}
