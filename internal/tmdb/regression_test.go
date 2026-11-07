package tmdb

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
)

type reviewFailTransport struct{}

func (reviewFailTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("simulated connection failure")
}

type errorTransport func(*http.Request) (*http.Response, error)

func (f errorTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) {
	return 0, errors.New("FAKE_REVIEW_KEY from upstream reader")
}
func (failingBody) Close() error { return nil }

func TestUpstreamErrorDetailsDoNotLeakKey(t *testing.T) {
	for _, response := range []*http.Response{
		{StatusCode: 502, Status: "502 FAKE_REVIEW_KEY", Body: io.NopCloser(strings.NewReader("FAKE_REVIEW_KEY"))},
		{StatusCode: 200, Body: failingBody{}},
	} {
		client, err := New(config.TMDBConfig{APIKey: "FAKE_REVIEW_KEY"})
		if err != nil {
			t.Fatal(err)
		}
		client.httpClient.Transport = errorTransport(func(*http.Request) (*http.Response, error) { return response, nil })
		_, err = client.Search(context.Background(), "movie", "movie", 2026)
		if err == nil || strings.Contains(err.Error(), "FAKE_REVIEW_KEY") {
			t.Fatalf("unsafe error: %v", err)
		}
	}
}
func TestRegressionTransportErrorDoesNotLeakKey(t *testing.T) {
	client, e := New(config.TMDBConfig{APIKey: "FAKE_REVIEW_KEY"})
	if e != nil {
		t.Fatal(e)
	}
	client.httpClient.Transport = reviewFailTransport{}
	_, err := client.Search(context.Background(), "movie", "movie", 2026)
	if err == nil {
		t.Fatal("expected transport failure")
	}
	if strings.Contains(err.Error(), "FAKE_REVIEW_KEY") {
		t.Fatal("transport error contains API key and is forwarded by searchTMDB")
	}
}
