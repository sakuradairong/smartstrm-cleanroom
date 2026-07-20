package storage

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/urlpolicy"
)

type ANiOpen struct {
	endpoint string
	client   *http.Client
	mu       sync.Mutex
	expires  time.Time
	entries  map[string][]Entry
	links    map[string]string
}

type aniRSS struct {
	Channel struct {
		Items []struct {
			Title   string `xml:"title"`
			Link    string `xml:"link"`
			PubDate string `xml:"pubDate"`
			Size    string `xml:"size"`
		} `xml:"item"`
	} `xml:"channel"`
}

func NewANiOpen(endpoint string) (*ANiOpen, error) {
	if endpoint == "" {
		endpoint = "https://api.ani.rip/ani-download.xml"
	}
	parsed, err := urlpolicy.ParseHTTP(endpoint, true)
	if err != nil {
		return nil, fmt.Errorf("invalid ANi Open endpoint: %w", err)
	}
	return &ANiOpen{endpoint: parsed.String(), client: &http.Client{Timeout: 20 * time.Second}, entries: make(map[string][]Entry), links: make(map[string]string)}, nil
}

func (a *ANiOpen) List(ctx context.Context, remotePath string) ([]Entry, error) {
	if err := a.refresh(ctx); err != nil {
		return nil, err
	}
	cleaned := CleanRemote(remotePath)
	a.mu.Lock()
	defer a.mu.Unlock()
	items, exists := a.entries[cleaned]
	if !exists {
		return nil, fmt.Errorf("ANi Open directory %q not found", cleaned)
	}
	return append([]Entry(nil), items...), nil
}

func (a *ANiOpen) DirectURL(ctx context.Context, remotePath string) (string, error) {
	if err := a.refresh(ctx); err != nil {
		return "", err
	}
	cleaned := CleanRemote(remotePath)
	a.mu.Lock()
	defer a.mu.Unlock()
	link, ok := a.links[cleaned]
	if !ok {
		return "", fmt.Errorf("ANi Open file %q not found", cleaned)
	}
	return link, nil
}

func (a *ANiOpen) Mkdir(context.Context, string) error { return fmt.Errorf("ANi Open is read-only") }
func (a *ANiOpen) Rename(context.Context, string, string) error {
	return fmt.Errorf("ANi Open is read-only")
}
func (a *ANiOpen) Move(context.Context, string, string) error {
	return fmt.Errorf("ANi Open is read-only")
}
func (a *ANiOpen) Delete(context.Context, string) error { return fmt.Errorf("ANi Open is read-only") }

func (a *ANiOpen) refresh(ctx context.Context) error {
	a.mu.Lock()
	if time.Now().Before(a.expires) && len(a.entries) > 0 {
		a.mu.Unlock()
		return nil
	}
	a.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ANi Open returned HTTP status %d", resp.StatusCode)
	}
	data, err := readBoundedResponseBody(resp, maximumMetadataResponseBytes)
	if err != nil {
		return fmt.Errorf("read ANi Open feed: %w", err)
	}
	var feed aniRSS
	if err := decodeStrictXML(data, &feed); err != nil {
		return fmt.Errorf("decode ANi Open feed: %w", err)
	}
	entries := map[string][]Entry{"/": {}}
	links := make(map[string]string)
	directories := make(map[string]time.Time)
	for _, item := range feed.Channel.Items {
		name := strings.TrimSpace(item.Title)
		if ValidateName(name) != nil {
			continue
		}
		link, err := urlpolicy.ParseHTTP(strings.TrimSpace(item.Link), true)
		if err != nil {
			continue
		}
		segments := strings.Split(strings.Trim(link.Path, "/"), "/")
		if len(segments) < 2 {
			continue
		}
		directory := segments[0]
		if ValidateName(directory) != nil {
			continue
		}
		modified, _ := time.Parse(time.RFC1123Z, strings.TrimSpace(item.PubDate))
		if modified.IsZero() {
			modified, _ = time.Parse(time.RFC1123, strings.TrimSpace(item.PubDate))
		}
		remote := CleanRemote("/" + directory + "/" + name)
		size := parseANiSize(item.Size)
		entries["/"+directory] = append(entries["/"+directory], Entry{Path: remote, Name: name, Size: size, ModTime: modified})
		links[remote] = link.String()
		if modified.After(directories[directory]) {
			directories[directory] = modified
		}
	}
	for directory, modified := range directories {
		entries["/"] = append(entries["/"], Entry{Path: "/" + directory, Name: directory, IsDir: true, ModTime: modified})
	}
	for key := range entries {
		sort.Slice(entries[key], func(i, j int) bool { return entries[key][i].Name < entries[key][j].Name })
	}
	a.mu.Lock()
	a.entries, a.links, a.expires = entries, links, time.Now().Add(5*time.Minute)
	a.mu.Unlock()
	return nil
}

func parseANiSize(value string) int64 {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) != 2 {
		return 0
	}
	number, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || number < 0 {
		return 0
	}
	multiplier := float64(1)
	switch strings.ToUpper(fields[1]) {
	case "KB":
		multiplier = 1 << 10
	case "MB":
		multiplier = 1 << 20
	case "GB":
		multiplier = 1 << 30
	case "TB":
		multiplier = 1 << 40
	default:
		return 0
	}
	return int64(number * multiplier)
}
