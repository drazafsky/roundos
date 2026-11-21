package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	latestReleaseURL = "https://api.github.com/repos/drazafsky/roundos/releases/latest"
	maxReleaseBytes  = 1024 * 1024
	requestTimeout   = 30 * time.Second
)

var ErrIncompatibleRelease = errors.New("stable release has no compatible ARMv7 package")

type CheckStatus string

const (
	UpToDate        CheckStatus = "up_to_date"
	Available       CheckStatus = "available"
	NoStableRelease CheckStatus = "no_stable_release"
)

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type Release struct {
	ID      int64
	Version string
	Tag     string
	Name    string
	Notes   string
	Assets  []Asset
}

type CheckResult struct {
	Status  CheckStatus
	Release *Release
}

type RateLimitError struct{ RetryAt time.Time }

func (e *RateLimitError) Error() string { return "GitHub release checking is rate limited" }

// GitHub discovers public stable releases without requiring credentials.
type GitHub struct {
	http     *http.Client
	endpoint string
	now      func() time.Time
}

func NewGitHub() *GitHub {
	return &GitHub{
		http: &http.Client{Timeout: requestTimeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.Host != "api.github.com" {
				return errors.New("unexpected GitHub API redirect")
			}
			return nil
		}},
		endpoint: latestReleaseURL,
		now:      time.Now,
	}
}

func (g *GitHub) Check(ctx context.Context, installed string) (CheckResult, error) {
	current, err := parseVersion(installed)
	if err != nil {
		return CheckResult{}, fmt.Errorf("installed version: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.endpoint, nil)
	if err != nil {
		return CheckResult{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "RoundOS/"+installed)
	resp, err := g.http.Do(req)
	if err != nil {
		return CheckResult{}, fmt.Errorf("check GitHub release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return CheckResult{Status: NoStableRelease}, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode == http.StatusForbidden &&
		(resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.Header.Get("Retry-After") != "")) {
		return CheckResult{}, &RateLimitError{RetryAt: retryDeadline(resp.Header, g.now())}
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden {
			// Secondary limits can return 403 without Retry-After and with
			// primary quota remaining. Do not expose the response body.
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
			var problem struct {
				Message string `json:"message"`
			}
			if readErr == nil && len(data) <= 64*1024 && json.Unmarshal(data, &problem) == nil {
				message := strings.ToLower(problem.Message)
				if strings.Contains(message, "rate limit") || strings.Contains(message, "abuse detection") {
					return CheckResult{}, &RateLimitError{RetryAt: retryDeadline(resp.Header, g.now())}
				}
			}
		}
		return CheckResult{}, fmt.Errorf("GitHub release check returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseBytes+1))
	if err != nil {
		return CheckResult{}, fmt.Errorf("read GitHub release: %w", err)
	}
	if len(data) > maxReleaseBytes {
		return CheckResult{}, errors.New("GitHub release response exceeds size limit")
	}
	var raw struct {
		ID         int64   `json:"id"`
		Tag        string  `json:"tag_name"`
		Name       string  `json:"name"`
		Notes      string  `json:"body"`
		Draft      bool    `json:"draft"`
		Prerelease bool    `json:"prerelease"`
		Assets     []Asset `json:"assets"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return CheckResult{}, fmt.Errorf("decode GitHub release: %w", err)
	}
	if raw.Draft || raw.Prerelease {
		return CheckResult{Status: NoStableRelease}, nil
	}
	candidate, err := parseVersion(raw.Tag)
	if err != nil || raw.ID <= 0 {
		return CheckResult{}, errors.New("GitHub returned an invalid release identity")
	}
	if candidate.pre != "" {
		return CheckResult{Status: NoStableRelease}, nil
	}
	if !newerStable(candidate, current) {
		return CheckResult{Status: UpToDate}, nil
	}
	v := strings.TrimPrefix(raw.Tag, "v")
	needed := map[string]bool{"roundos-" + v + "-linux-armv7.tar.gz": false, "release.json": false, "SHA256SUMS": false}
	var assets []Asset
	for _, a := range raw.Assets {
		seen, required := needed[a.Name]
		if !required {
			continue
		}
		if seen || a.Size <= 0 || !validAssetURL(a.URL, raw.Tag, a.Name) {
			return CheckResult{}, ErrIncompatibleRelease
		}
		needed[a.Name] = true
		assets = append(assets, a)
	}
	for _, present := range needed {
		if !present {
			return CheckResult{}, ErrIncompatibleRelease
		}
	}
	return CheckResult{Status: Available, Release: &Release{ID: raw.ID, Version: v, Tag: raw.Tag, Name: raw.Name, Notes: raw.Notes, Assets: assets}}, nil
}

func validAssetURL(raw, tag, name string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "github.com" && u.User == nil && u.RawQuery == "" && u.Fragment == "" &&
		u.Path == "/drazafsky/roundos/releases/download/"+tag+"/"+name
}

func retryDeadline(h http.Header, now time.Time) time.Time {
	// A response without a usable deadline receives a conservative one-hour
	// backoff. Honor the latest usable deadline when both headers are present.
	var deadline time.Time
	if seconds, err := strconv.ParseInt(h.Get("Retry-After"), 10, 64); err == nil && seconds > 0 && seconds <= int64((time.Duration(1<<63-1))/time.Second) {
		deadline = now.Add(time.Duration(seconds) * time.Second)
	} else if at, err := http.ParseTime(h.Get("Retry-After")); err == nil && at.After(now) {
		deadline = at
	}
	if seconds, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		if at := time.Unix(seconds, 0); at.After(deadline) && at.After(now) {
			deadline = at
		}
	}
	if !deadline.After(now) {
		deadline = now.Add(time.Hour)
	}
	return deadline
}
