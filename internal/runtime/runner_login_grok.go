package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

const grokAuthFile = "auth.json"

// grokRefreshFields are the only fields a token refresh may change. Every other
// field identifies the account, so a sandbox cannot swap the host login for
// another account by editing its copy.
var grokRefreshFields = map[string]bool{"key": true, "refresh_token": true, "expires_at": true}

// parseGrokLogin reads ~/.grok/auth.json: one object per issuer::client entry.
func parseGrokLogin(data []byte) (map[string]map[string]any, error) {
	var login map[string]map[string]any
	if err := json.Unmarshal(data, &login); err != nil || len(login) == 0 {
		return nil, fmt.Errorf("invalid Grok login")
	}
	for _, entry := range login {
		if entry == nil || entry["refresh_token"] == nil || entry["user_id"] == nil {
			return nil, fmt.Errorf("invalid Grok login")
		}
	}
	return login, nil
}

// sameGrokLogin accepts refreshed only when it holds the same entries and
// differs from original in the refresh fields alone.
func sameGrokLogin(original, refreshed []byte) bool {
	before, err := parseGrokLogin(original)
	if err != nil {
		return false
	}
	after, err := parseGrokLogin(refreshed)
	if err != nil || len(after) != len(before) {
		return false
	}
	for id, old := range before {
		updated, ok := after[id]
		if !ok || len(updated) != len(old) {
			return false
		}
		for key, value := range old {
			next, ok := updated[key]
			if !ok || (!grokRefreshFields[key] && !reflect.DeepEqual(value, next)) {
				return false
			}
		}
	}
	return true
}

// readGrokRunnerLogin copies the machine owner's `grok login`. Grok rotates
// its refresh token, so a refresh made inside the sandbox is written back to
// the host when it belongs to the same account and the host file is unchanged.
func readGrokRunnerLogin() ([]byte, func([]byte) error, error) {
	dir := os.Getenv("GROK_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, err
		}
		dir = filepath.Join(home, ".grok")
	}
	original, err := readRunnerLoginFile(dir, grokAuthFile)
	if err != nil {
		return nil, nil, err
	}
	if _, err := parseGrokLogin(original); err != nil {
		return nil, nil, err
	}
	return original, func(refreshed []byte) error {
		if bytes.Equal(original, refreshed) {
			return nil
		}
		if !sameGrokLogin(original, refreshed) {
			return fmt.Errorf("refreshed Grok login does not match the runner account")
		}
		return replaceRunnerLoginFile(dir, grokAuthFile, original, refreshed)
	}, nil
}
