package benchtask

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Profile is a user profile as returned by the profile service.
type Profile struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ProfileErrKind classifies a ProfileError.
type ProfileErrKind int

// ProfileErrKind values.
const (
	ProfileErrFetch ProfileErrKind = iota + 1
	ProfileErrDecode
)

// ProfileError is returned by FetchProfile. For ProfileErrFetch, Err is a
// *FetchError; for ProfileErrDecode, Err is the JSON decoding error.
type ProfileError struct {
	Kind   ProfileErrKind
	UserID int64
	Err    error
}

func (e *ProfileError) Error() string {
	var what string
	switch e.Kind {
	case ProfileErrFetch:
		what = "fetch"
	case ProfileErrDecode:
		what = "decode"
	default:
		what = "ProfileErrKind(" + strconv.Itoa(int(e.Kind)) + ")"
	}
	return fmt.Sprintf("profile %d: %s: %v", e.UserID, what, e.Err)
}

// Unwrap returns the underlying cause.
func (e *ProfileError) Unwrap() error { return e.Err }

// FetchProfile retrieves and decodes the profile at baseURL/users/{userID}.
func FetchProfile(ctx context.Context, doer Doer, clock Clock, rnd Rand, baseURL string, userID int64, policy RetryPolicy) (Profile, error) {
	target := strings.TrimRight(baseURL, "/") + "/users/" + strconv.FormatInt(userID, 10)
	body, err := GetWithRetry(ctx, doer, clock, rnd, target, policy)
	if err != nil {
		return Profile{}, &ProfileError{Kind: ProfileErrFetch, UserID: userID, Err: err}
	}
	var p Profile
	if err := json.Unmarshal(body, &p); err != nil {
		return Profile{}, &ProfileError{Kind: ProfileErrDecode, UserID: userID, Err: err}
	}
	return p, nil
}
