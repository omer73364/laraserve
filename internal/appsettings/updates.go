package appsettings

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Masterminds/semver/v3"
	"laraserve/internal/projectconfig"
	"laraserve/internal/statefile"
)

const ReleasesURL = "https://github.com/omer73364/laraserve/releases"
const releaseAPI = "https://api.github.com/repos/omer73364/laraserve/releases/latest"

type Update struct {
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
	Available bool   `json:"available"`
	Checked   string `json:"checked,omitempty"`
	Message   string `json:"message"`
	URL       string `json:"url"`
}

func ReadUpdate(home, current string) Update {
	result := Update{Current: current, URL: ReleasesURL, Message: "Updates have not been checked."}
	b, err := projectconfig.ReadRegular(filepath.Join(home, "update-check.json"), 64<<10)
	if err == nil {
		_ = json.Unmarshal(b, &result)
	}
	result.Current = current
	result.Available = false
	if latest, e := semver.NewVersion(result.Latest); e == nil {
		if now, e := semver.NewVersion(current); e == nil {
			result.Available = latest.GreaterThan(now)
		}
	}
	result.URL = ReleasesURL
	return result
}
func Check(ctx context.Context, home, current string, force bool) (Update, error) {
	result := ReadUpdate(home, current)
	if when, err := time.Parse(time.RFC3339, result.Checked); !force && err == nil && time.Since(when) >= 0 && time.Since(when) < 24*time.Hour {
		return result, nil
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	result, err := fetchRelease(ctx, client, releaseAPI, current)
	if err != nil {
		return result, err
	}
	return result, statefile.WriteJSON(filepath.Join(home, "update-check.json"), result)
}
func fetchRelease(ctx context.Context, client *http.Client, url, current string) (Update, error) {
	result := Update{Current: current, URL: ReleasesURL, Checked: time.Now().UTC().Format(time.RFC3339)}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "LaraServe/"+current)
	response, err := client.Do(req)
	if err != nil {
		return result, fmt.Errorf("could not check GitHub releases: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == 404 {
		result.Message = "No published release is available yet."
		return result, nil
	}
	if response.StatusCode != 200 {
		return result, fmt.Errorf("release check returned HTTP %d; try again later", response.StatusCode)
	}
	var release struct {
		Tag string `json:"tag_name"`
	}
	b, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return result, err
	}
	if err = json.Unmarshal(b, &release); err != nil {
		return result, fmt.Errorf("invalid release metadata")
	}
	latest, err := semver.NewVersion(release.Tag)
	if err != nil {
		return result, fmt.Errorf("release has an invalid version")
	}
	result.Latest = release.Tag
	now, err := semver.NewVersion(current)
	if err != nil {
		result.Message = "Latest published version found. This development build cannot be compared automatically."
		return result, nil
	}
	result.Available = latest.GreaterThan(now)
	result.Message = "You are using the latest published version or a newer build."
	if result.Available {
		result.Message = "A newer release is available. Review it before updating."
	}
	return result, nil
}
