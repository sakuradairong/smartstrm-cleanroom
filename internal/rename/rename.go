package rename

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/storage"
)

type Change struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Options struct {
	Mode              string
	Pattern           string
	Replacement       string
	Template          string
	Prefix            string
	Suffix            string
	Start             int
	Width             int
	PreserveExtension bool
}

func Plan(entries []storage.Entry, options Options) ([]Change, error) {
	files := make([]storage.Entry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir {
			files = append(files, entry)
		}
	}
	sort.Slice(files, func(i, j int) bool { return naturalLess(files[i].Name, files[j].Name) })
	changes := make([]Change, 0, len(files))
	switch options.Mode {
	case "regex":
		re, err := regexp.Compile(options.Pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid rename regex: %w", err)
		}
		for _, file := range files {
			name := re.ReplaceAllString(file.Name, options.Replacement)
			if name != file.Name {
				changes = append(changes, Change{From: file.Name, To: name})
			}
		}
	case "sequence":
		if options.Width < 1 || options.Width > 12 {
			return nil, fmt.Errorf("sequence width must be 1-12")
		}
		for index, file := range files {
			ext := ""
			if options.PreserveExtension {
				ext = path.Ext(file.Name)
			}
			name := options.Prefix + fmt.Sprintf("%0*d", options.Width, options.Start+index) + options.Suffix + ext
			if name != file.Name {
				changes = append(changes, Change{From: file.Name, To: name})
			}
		}
	case "magic":
		if options.Template == "" {
			return nil, fmt.Errorf("magic template is required")
		}
		for _, file := range files {
			metadata, ok := parseMetadata(file.Name)
			if !ok {
				continue
			}
			name := options.Template
			for key, value := range metadata {
				name = strings.ReplaceAll(name, "{"+key+"}", value)
			}
			if name != file.Name {
				changes = append(changes, Change{From: file.Name, To: name})
			}
		}
	default:
		return nil, fmt.Errorf("unsupported rename mode %q", options.Mode)
	}
	validated, err := validate(changes, entries)
	if err != nil {
		return nil, fmt.Errorf("validate rename plan: %w", err)
	}
	return validated, nil
}

func Execute(ctx context.Context, instance storage.Storage, directory string, changes []Change) error {
	if len(changes) == 0 {
		return nil
	}
	nonceBytes := make([]byte, 8)
	if _, err := rand.Read(nonceBytes); err != nil {
		return fmt.Errorf("create rename transaction id: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)
	temporary := make([]string, len(changes))
	for index := range changes {
		temporary[index] = fmt.Sprintf(".smartstrm-%s-%d", nonce, index)
	}

	staged := 0
	for index, change := range changes {
		if err := instance.Rename(ctx, path.Join(directory, change.From), temporary[index]); err != nil {
			rollbackStaged(ctx, instance, directory, changes, temporary, staged)
			return fmt.Errorf("stage rename %q: %w", change.From, err)
		}
		staged++
	}
	finalized := 0
	for index, change := range changes {
		if err := instance.Rename(ctx, path.Join(directory, temporary[index]), change.To); err != nil {
			for done := 0; done < finalized; done++ {
				_ = instance.Rename(ctx, path.Join(directory, changes[done].To), temporary[done])
			}
			rollbackStaged(ctx, instance, directory, changes, temporary, len(changes))
			return fmt.Errorf("finalize rename %q to %q: %w", change.From, change.To, err)
		}
		finalized++
	}
	return nil
}

func rollbackStaged(ctx context.Context, instance storage.Storage, directory string, changes []Change, temporary []string, count int) {
	for index := count - 1; index >= 0; index-- {
		_ = instance.Rename(ctx, path.Join(directory, temporary[index]), changes[index].From)
	}
}

var episodePattern = regexp.MustCompile(`(?i)^(.*?)[ ._-]*(?:S(\d{1,3})[ ._-]*)?E(?:P)?(\d{1,4})(.*)$`)

func parseMetadata(name string) (map[string]string, bool) {
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	match := episodePattern.FindStringSubmatch(base)
	if match == nil {
		return nil, false
	}
	season := match[2]
	if season == "" {
		season = "1"
	}
	title := strings.Trim(strings.ReplaceAll(match[1], ".", " "), " _-")
	if title == "" {
		title = "Episode"
	}
	seasonNumber, _ := strconv.Atoi(season)
	episodeNumber, _ := strconv.Atoi(match[3])
	return map[string]string{
		"title": title, "season": fmt.Sprintf("%02d", seasonNumber), "episode": fmt.Sprintf("%02d", episodeNumber), "ext": ext,
	}, true
}

func EpisodeMetadata(name string) (map[string]string, bool) { return parseMetadata(name) }

func validate(changes []Change, entries []storage.Entry) ([]Change, error) {
	existing := make(map[string]bool, len(entries))
	moving := make(map[string]bool, len(changes))
	for _, entry := range entries {
		existing[entry.Name] = true
	}
	for _, change := range changes {
		moving[change.From] = true
	}
	targets := make(map[string]bool, len(changes))
	for _, change := range changes {
		if err := storage.ValidateName(change.To); err != nil {
			return nil, err
		}
		if targets[change.To] {
			return nil, fmt.Errorf("duplicate target name %q", change.To)
		}
		if existing[change.To] && !moving[change.To] {
			return nil, fmt.Errorf("target name %q already exists", change.To)
		}
		targets[change.To] = true
	}
	return changes, nil
}

func Validate(changes []Change, entries []storage.Entry) ([]Change, error) {
	validated, err := validate(changes, entries)
	if err != nil {
		return nil, fmt.Errorf("validate rename changes: %w", err)
	}
	return validated, nil
}

var numberPattern = regexp.MustCompile(`\d+|\D+`)

func naturalLess(left, right string) bool {
	a, b := numberPattern.FindAllString(left, -1), numberPattern.FindAllString(right, -1)
	for index := 0; index < len(a) && index < len(b); index++ {
		leftNumber, leftErr := strconv.Atoi(a[index])
		rightNumber, rightErr := strconv.Atoi(b[index])
		if leftErr == nil && rightErr == nil && leftNumber != rightNumber {
			return leftNumber < rightNumber
		}
		if a[index] != b[index] {
			return strings.ToLower(a[index]) < strings.ToLower(b[index])
		}
	}
	return len(a) < len(b)
}
