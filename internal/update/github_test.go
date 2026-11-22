package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func releaseFixture() map[string]any {
	var assets []Asset
	for _, name := range []string{"roundos-0.6.0-linux-armv7.tar.gz", "release.json", "SHA256SUMS"} {
		assets = append(assets, Asset{Name: name, Size: 123, URL: "https://github.com/drazafsky/roundos/releases/download/v0.6.0/" + name})
	}
	return map[string]any{"id": 123, "tag_name": "v0.6.0", "name": "RoundOS 0.6.0", "body": "## Changes\nLonger release notes.", "draft": false, "prerelease": false, "assets": assets}
}

func testGitHub(t *testing.T, handler http.HandlerFunc) *GitHub {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	g := NewGitHub()
	g.endpoint = server.URL + "/repos/drazafsky/roundos/releases/latest"
	g.now = func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC) }
	return g
}

func TestGitHubStableDiscoveryAndRetainedNotes(t *testing.T) {
	requests := 0
	g := testGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != "GET" || r.URL.Path != "/repos/drazafsky/roundos/releases/latest" || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != "RoundOS/0.5.1-dev" || r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("unexpected GitHub request: %+v", r)
		}
		json.NewEncoder(w).Encode(releaseFixture())
	})
	r, err := g.Check(context.Background(), "0.5.1-dev")
	if err != nil || r.Status != Available || r.Release == nil {
		t.Fatalf("discovery: %+v, %v", r, err)
	}
	if r.Release.ID != 123 || r.Release.Version != "0.6.0" || r.Release.Tag != "v0.6.0" || len(r.Release.Assets) != 3 {
		t.Fatalf("lost release identity: %+v", r.Release)
	}
	for range 3 {
		if r.Release.Notes != "## Changes\nLonger release notes." {
			t.Fatal("release notes were not retained")
		}
	}
	if requests != 1 {
		t.Fatalf("notes caused additional checks: %d", requests)
	}
}

func TestGitHubVersionOrdering(t *testing.T) {
	for _, tc := range []struct {
		installed string
		want      CheckStatus
	}{
		{"0.5.9", Available}, {"0.6.0-dev.42.gabcdef0", Available},
		{"v0.6.0-rc.1", Available}, {"0.6.0", UpToDate},
		{"0.6.0+build.123", UpToDate}, {"0.6.1-dev", UpToDate},
		{"0.10.0", UpToDate}, {"1.0.0", UpToDate},
	} {
		t.Run(tc.installed, func(t *testing.T) {
			g := testGitHub(t, func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(releaseFixture()) })
			r, err := g.Check(context.Background(), tc.installed)
			if err != nil || r.Status != tc.want {
				t.Fatalf("version ordering: %+v, %v", r, err)
			}
		})
	}
}

func TestGitHubUnavailableAndInvalidReleases(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
		want   CheckStatus
		bad    bool
	}{
		{"draft", func(f map[string]any) { f["draft"] = true }, NoStableRelease, false},
		{"prerelease flag", func(f map[string]any) { f["prerelease"] = true }, NoStableRelease, false},
		{"prerelease version", func(f map[string]any) { f["tag_name"] = "v0.6.0-rc.1" }, NoStableRelease, false},
		{"malformed version", func(f map[string]any) { f["tag_name"] = "stable" }, "", true},
		{"missing id", func(f map[string]any) { delete(f, "id") }, "", true},
		{"missing assets", func(f map[string]any) { f["assets"] = []Asset{} }, "", true},
		{"missing package", func(f map[string]any) { f["assets"] = f["assets"].([]Asset)[1:] }, "", true},
		{"duplicate asset", func(f map[string]any) { a := f["assets"].([]Asset); f["assets"] = append(a, a[0]) }, "", true},
		{"foreign URL", func(f map[string]any) { f["assets"].([]Asset)[0].URL = "https://example.com/package.tar.gz" }, "", true},
		{"wrong tag URL", func(f map[string]any) {
			a := f["assets"].([]Asset)
			a[0].URL = strings.ReplaceAll(a[0].URL, "v0.6.0", "v0.5.0")
		}, "", true},
		{"empty asset", func(f map[string]any) { f["assets"].([]Asset)[0].Size = 0 }, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := testGitHub(t, func(w http.ResponseWriter, r *http.Request) {
				f := releaseFixture()
				tc.mutate(f)
				json.NewEncoder(w).Encode(f)
			})
			r, err := g.Check(context.Background(), "0.5.1")
			if (err != nil) != tc.bad || r.Status != tc.want || r.Release != nil {
				t.Fatalf("unexpected discovery result: %+v, %v", r, err)
			}
		})
	}
}

func TestGitHubHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		body string
		bad  bool
	}{
		{"no stable release", 404, "not found", false},
		{"server error", 503, "unavailable", true},
		{"forbidden", 403, "forbidden", true},
		{"malformed", 200, "{", true},
		{"null", 200, "null", true},
		{"multiple documents", 200, "{} {}", true},
		{"too large", 200, strings.Repeat(" ", maxReleaseBytes+1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := testGitHub(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.code); w.Write([]byte(tc.body)) })
			r, err := g.Check(context.Background(), "0.5.1")
			if (err != nil) != tc.bad || (!tc.bad && r.Status != NoStableRelease) {
				t.Fatalf("response handling: %+v, %v", r, err)
			}
		})
	}
}

func TestGitHubRateLimitDeadlines(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		code    int
		headers map[string]string
		wait    time.Duration
	}{
		{"seconds", 429, map[string]string{"Retry-After": "1200"}, 20 * time.Minute},
		{"HTTP date", 429, map[string]string{"Retry-After": now.Add(30 * time.Minute).Format(http.TimeFormat)}, 30 * time.Minute},
		{"primary", 403, map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": strconv.FormatInt(now.Add(40*time.Minute).Unix(), 10)}, 40 * time.Minute},
		{"later deadline", 429, map[string]string{"Retry-After": "1200", "X-RateLimit-Reset": strconv.FormatInt(now.Add(time.Hour).Unix(), 10)}, time.Hour},
		{"missing deadline", 429, nil, time.Hour},
		{"secondary without headers", 403, map[string]string{"X-RateLimit-Remaining": "50"}, time.Hour},
		{"invalid deadline", 429, map[string]string{"Retry-After": "9999999999999999999999999"}, time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := testGitHub(t, func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tc.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tc.code)
				if tc.name == "secondary without headers" {
					w.Write([]byte(`{"message":"You have exceeded a secondary rate limit. Please wait a few minutes before you try again."}`))
				}
			})
			_, err := g.Check(context.Background(), "0.5.1")
			var limited *RateLimitError
			if !errors.As(err, &limited) || !limited.RetryAt.Equal(now.Add(tc.wait)) || !limited.ObservedAt.Equal(now) {
				t.Fatalf("rate limit: %v", err)
			}
		})
	}
}

func TestGitHubCancellationAndNetworkFailure(t *testing.T) {
	g := testGitHub(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := g.Check(ctx, "0.5.1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("request did not honor cancellation: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()
	g.endpoint = server.URL
	if _, err := g.Check(context.Background(), "0.5.1"); err == nil {
		t.Fatal("network failure reported success")
	}
	if _, err := g.Check(context.Background(), "unknown"); err == nil {
		t.Fatal("unversioned installed build reported success")
	}
}

func TestGitHubRejectsUnexpectedRedirect(t *testing.T) {
	g := NewGitHub()
	u, _ := http.NewRequest("GET", "http://example.com/release", nil)
	if err := g.http.CheckRedirect(u, nil); err == nil {
		t.Fatal("allowed a foreign insecure API redirect")
	}
}
