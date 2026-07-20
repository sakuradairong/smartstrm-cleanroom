package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
)

type Client struct {
	apiKey, language, baseURL, imageBaseURL string
	httpClient                              *http.Client
	imageClient                             *http.Client
	cacheTTL                                time.Duration
	cacheMu                                 sync.RWMutex
	cache                                   map[string]cacheEntry
}

type cacheEntry struct {
	data    []byte
	expires time.Time
}

type SearchResult struct {
	ID            int     `json:"id"`
	MediaType     string  `json:"media_type"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	ReleaseDate   string  `json:"release_date"`
	PosterPath    string  `json:"poster_path"`
	Overview      string  `json:"overview"`
	VoteAverage   float64 `json:"vote_average"`
}

type Details struct {
	ID            int     `json:"id"`
	MediaType     string  `json:"media_type"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	ReleaseDate   string  `json:"release_date"`
	PosterPath    string  `json:"poster_path"`
	Overview      string  `json:"overview"`
	VoteAverage   float64 `json:"vote_average"`
}

type EpisodeDetails struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	Overview      string  `json:"overview"`
	AirDate       string  `json:"air_date"`
	StillPath     string  `json:"still_path"`
	VoteAverage   float64 `json:"vote_average"`
	SeasonNumber  int     `json:"season_number"`
	EpisodeNumber int     `json:"episode_number"`
}

func New(cfg config.TMDBConfig) (*Client, error) {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.themoviedb.org/3"
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("invalid TMDB base URL")
	}
	imageBaseURL := strings.TrimRight(cfg.ImageBaseURL, "/")
	if imageBaseURL == "" {
		imageBaseURL = "https://image.tmdb.org/t/p/w185"
	}
	imageBase, err := url.Parse(imageBaseURL)
	if err != nil || (imageBase.Scheme != "http" && imageBase.Scheme != "https") || imageBase.Host == "" || imageBase.User != nil || imageBase.RawQuery != "" || imageBase.Fragment != "" {
		return nil, fmt.Errorf("invalid TMDB image base URL")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.ProxyURL != "" {
		proxyURL, err := url.Parse(cfg.ProxyURL)
		if err != nil || proxyURL.Scheme == "" || proxyURL.Host == "" {
			return nil, fmt.Errorf("invalid TMDB proxy URL")
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	language := cfg.Language
	if language == "" {
		language = "zh-CN"
	}
	cacheTTL := time.Duration(cfg.CacheMinutes) * time.Minute
	if cacheTTL <= 0 {
		cacheTTL = 30 * time.Minute
	}
	return &Client{apiKey: cfg.APIKey, language: language, baseURL: baseURL, imageBaseURL: imageBaseURL, httpClient: &http.Client{Transport: transport, Timeout: 20 * time.Second}, imageClient: &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, cacheTTL: cacheTTL, cache: make(map[string]cacheEntry)}, nil
}

func (c *Client) Enabled() bool { return c.apiKey != "" }

func (c *Client) Image(ctx context.Context, posterPath string) ([]byte, string, error) {
	name := strings.TrimPrefix(strings.TrimSpace(posterPath), "/")
	if name == "" || path.Base(name) != name || strings.ContainsAny(name, `\?#`) {
		return nil, "", fmt.Errorf("invalid TMDB image path")
	}
	extension := strings.ToLower(path.Ext(name))
	if extension != ".jpg" && extension != ".jpeg" && extension != ".png" && extension != ".webp" {
		return nil, "", fmt.Errorf("unsupported TMDB image extension")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.imageBaseURL+"/"+url.PathEscape(name), nil)
	if err != nil {
		return nil, "", err
	}
	response, err := c.imageClient.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", fmt.Errorf("TMDB image returned %s", response.Status)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" {
		return nil, "", fmt.Errorf("TMDB image returned unsupported content type")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 5<<20+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 || len(data) > 5<<20 {
		return nil, "", fmt.Errorf("TMDB image size is invalid")
	}
	return data, contentType, nil
}

func (c *Client) Search(ctx context.Context, query, mediaType string, year int) ([]SearchResult, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("TMDB API key is not configured")
	}
	endpoint := "/search/multi"
	if mediaType == "movie" || mediaType == "tv" {
		endpoint = "/search/" + mediaType
	}
	values := url.Values{"query": {query}, "include_adult": {"false"}}
	if year > 0 {
		if mediaType == "tv" {
			values.Set("first_air_date_year", fmt.Sprint(year))
		} else {
			values.Set("year", fmt.Sprint(year))
		}
	}
	var payload struct {
		Results []struct {
			ID            int     `json:"id"`
			MediaType     string  `json:"media_type"`
			Title         string  `json:"title"`
			Name          string  `json:"name"`
			OriginalTitle string  `json:"original_title"`
			OriginalName  string  `json:"original_name"`
			ReleaseDate   string  `json:"release_date"`
			FirstAirDate  string  `json:"first_air_date"`
			PosterPath    string  `json:"poster_path"`
			Overview      string  `json:"overview"`
			VoteAverage   float64 `json:"vote_average"`
		} `json:"results"`
	}
	if err := c.get(ctx, endpoint, values, &payload); err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(payload.Results))
	for _, item := range payload.Results {
		typeName := item.MediaType
		if typeName == "" {
			typeName = mediaType
		}
		if typeName != "movie" && typeName != "tv" {
			continue
		}
		title, original, date := item.Title, item.OriginalTitle, item.ReleaseDate
		if typeName == "tv" {
			title, original, date = item.Name, item.OriginalName, item.FirstAirDate
		}
		if original == "" {
			original = title
		}
		results = append(results, SearchResult{ID: item.ID, MediaType: typeName, Title: title, OriginalTitle: original, ReleaseDate: date, PosterPath: item.PosterPath, Overview: item.Overview, VoteAverage: item.VoteAverage})
	}
	return results, nil
}

func (c *Client) Details(ctx context.Context, mediaType string, id int) (Details, error) {
	if !c.Enabled() {
		return Details{}, fmt.Errorf("TMDB API key is not configured")
	}
	if mediaType != "movie" && mediaType != "tv" {
		return Details{}, fmt.Errorf("media type must be movie or tv")
	}
	var payload struct {
		ID            int     `json:"id"`
		Title         string  `json:"title"`
		Name          string  `json:"name"`
		OriginalTitle string  `json:"original_title"`
		OriginalName  string  `json:"original_name"`
		ReleaseDate   string  `json:"release_date"`
		FirstAirDate  string  `json:"first_air_date"`
		PosterPath    string  `json:"poster_path"`
		Overview      string  `json:"overview"`
		VoteAverage   float64 `json:"vote_average"`
	}
	if err := c.get(ctx, path.Join("/", mediaType, fmt.Sprint(id)), nil, &payload); err != nil {
		return Details{}, err
	}
	title, original, date := payload.Title, payload.OriginalTitle, payload.ReleaseDate
	if mediaType == "tv" {
		title, original, date = payload.Name, payload.OriginalName, payload.FirstAirDate
	}
	if original == "" {
		original = title
	}
	return Details{ID: payload.ID, MediaType: mediaType, Title: title, OriginalTitle: original, ReleaseDate: date, PosterPath: payload.PosterPath, Overview: payload.Overview, VoteAverage: payload.VoteAverage}, nil
}

func (c *Client) Episode(ctx context.Context, tvID, season, episode int) (EpisodeDetails, error) {
	if !c.Enabled() {
		return EpisodeDetails{}, fmt.Errorf("TMDB API key is not configured")
	}
	if tvID < 1 || season < 0 || episode < 1 {
		return EpisodeDetails{}, fmt.Errorf("invalid TV episode coordinates")
	}
	var result EpisodeDetails
	if err := c.get(ctx, fmt.Sprintf("/tv/%d/season/%d/episode/%d", tvID, season, episode), nil, &result); err != nil {
		return EpisodeDetails{}, err
	}
	return result, nil
}

func (c *Client) get(ctx context.Context, endpoint string, values url.Values, target any) error {
	if values == nil {
		values = make(url.Values)
	}
	values.Set("api_key", c.apiKey)
	values.Set("language", c.language)
	requestURL := c.baseURL + endpoint + "?" + values.Encode()
	if data, ok := c.cached(requestURL); ok {
		return json.Unmarshal(data, target)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("TMDB returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return err
	}
	c.cacheMu.Lock()
	c.cache[requestURL] = cacheEntry{data: append([]byte(nil), data...), expires: time.Now().Add(c.cacheTTL)}
	c.cacheMu.Unlock()
	return nil
}

func (c *Client) cached(key string) ([]byte, bool) {
	c.cacheMu.RLock()
	item, ok := c.cache[key]
	c.cacheMu.RUnlock()
	if !ok || time.Now().After(item.expires) {
		if ok {
			c.cacheMu.Lock()
			delete(c.cache, key)
			c.cacheMu.Unlock()
		}
		return nil, false
	}
	return append([]byte(nil), item.data...), true
}
