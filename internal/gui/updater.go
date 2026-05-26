package gui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	releaseURL       = "https://api.github.com/repos/bhayanak/swiftload-downloader/releases/latest"
	prefLastUpdateCk = "last_update_check"
	updateCheckEvery = 24 * time.Hour
)

// ReleaseInfo holds the parsed latest release data.
type ReleaseInfo struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Name    string `json:"name"`
}

// CheckForUpdate queries GitHub for the latest release and returns info if newer.
// Returns nil, nil if no update is available or an error occurs silently.
func CheckForUpdate(currentVersion string) (*ReleaseInfo, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(releaseURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var info ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	if isNewer(info.TagName, currentVersion) {
		return &info, nil
	}
	return nil, nil
}

// isNewer returns true if remote version tag is greater than local.
func isNewer(remoteTag, localVersion string) bool {
	remote := parseVersion(remoteTag)
	local := parseVersion(localVersion)
	if remote == nil || local == nil {
		return false
	}
	for i := 0; i < 3; i++ {
		if remote[i] > local[i] {
			return true
		}
		if remote[i] < local[i] {
			return false
		}
	}
	return false
}

// parseVersion extracts [major, minor, patch] from a version string like "v2.1.0" or "2.1.0".
func parseVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	// Ignore pre-release tags.
	if strings.Contains(v, "-") {
		return nil
	}
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return nil
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		nums[i] = n
	}
	return nums
}
