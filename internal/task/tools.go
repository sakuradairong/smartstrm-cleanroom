package task

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ReplaceResult struct {
	Scanned int `json:"scanned"`
	Changed int `json:"changed"`
}

type CoverOptions struct {
	Binary    string
	PublicURL string
	Subpath   string
	Position  time.Duration
	Timeout   time.Duration
	Overwrite bool
	Preview   bool
}

type CoverChange struct {
	STRM  string `json:"strm"`
	Cover string `json:"cover"`
}

type CoverResult struct {
	Scanned int           `json:"scanned"`
	Planned int           `json:"planned"`
	Created int           `json:"created"`
	Skipped int           `json:"skipped"`
	Changes []CoverChange `json:"changes"`
}

func ExtractVideoCovers(ctx context.Context, root string, options CoverOptions) (CoverResult, error) {
	root, err := safeToolSubroot(root, options.Subpath)
	if err != nil {
		return CoverResult{}, err
	}
	if options.Binary == "" {
		options.Binary = "ffmpeg"
	}
	if options.Timeout <= 0 || options.Timeout > 10*time.Minute {
		return CoverResult{}, fmt.Errorf("ffmpeg timeout must be between 1ns and 10m")
	}
	publicURL, err := url.Parse(options.PublicURL)
	if err != nil || (publicURL.Scheme != "http" && publicURL.Scheme != "https") || publicURL.Host == "" {
		return CoverResult{}, fmt.Errorf("invalid public URL")
	}
	streamPrefix := strings.TrimRight(publicURL.EscapedPath(), "/") + "/stream/"
	result := CoverResult{Changes: make([]CoverChange, 0)}
	err = filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".strm") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 64<<10 {
			return nil
		}
		result.Scanned++
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		input := strings.TrimSpace(string(data))
		if err := validateCoverInput(input, publicURL, streamPrefix); err != nil {
			return fmt.Errorf("%s: %w", current, err)
		}
		cover := strings.TrimSuffix(current, filepath.Ext(current)) + ".jpg"
		if target, statErr := os.Lstat(cover); statErr == nil {
			if target.Mode()&os.ModeSymlink != 0 || !target.Mode().IsRegular() {
				return fmt.Errorf("cover target %q must be a regular file", cover)
			}
			if !options.Overwrite {
				result.Skipped++
				return nil
			}
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		result.Planned++
		result.Changes = append(result.Changes, CoverChange{STRM: current, Cover: cover})
		if options.Preview {
			return nil
		}
		if err := extractCover(ctx, input, cover, options); err != nil {
			return fmt.Errorf("extract %s: %w", current, err)
		}
		result.Created++
		return nil
	})
	return result, err
}

func safeToolSubroot(root, subpath string) (string, error) {
	root, err := safeToolRoot(root)
	if err != nil {
		return "", fmt.Errorf("validate task destination: %w", err)
	}
	if strings.TrimSpace(subpath) == "" || subpath == "/" {
		return root, nil
	}
	relative := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(subpath, "/")))
	if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe task subpath %q", subpath)
	}
	target := filepath.Join(root, relative)
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve task destination: %w", err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("resolve task subpath: %w", err)
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedTarget)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("task subpath escapes destination")
	}
	validated, err := safeToolRoot(resolvedTarget)
	if err != nil {
		return "", fmt.Errorf("validate resolved task subpath: %w", err)
	}
	return validated, nil
}

func validateCoverInput(input string, publicURL *url.URL, streamPrefix string) error {
	parsed, err := url.Parse(input)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("STRM content is not an absolute URL")
	}
	if !strings.EqualFold(parsed.Scheme, publicURL.Scheme) || !strings.EqualFold(parsed.Host, publicURL.Host) {
		return fmt.Errorf("STRM URL must use configured public origin")
	}
	if parsed.User != nil || parsed.Fragment != "" || !strings.HasPrefix(parsed.EscapedPath(), streamPrefix) {
		return fmt.Errorf("STRM URL must be a signed /stream/ address")
	}
	return nil
}

func extractCover(parent context.Context, input, cover string, options CoverOptions) error {
	ctx, cancel := context.WithTimeout(parent, options.Timeout)
	defer cancel()
	directory := filepath.Dir(cover)
	temporary, err := os.CreateTemp(directory, ".smartstrm-cover-*.jpg")
	if err != nil {
		return fmt.Errorf("create temporary cover: %w", err)
	}
	temporaryName := temporary.Name()
	if err := temporary.Close(); err != nil {
		os.Remove(temporaryName)
		return fmt.Errorf("close temporary cover: %w", err)
	}
	defer os.Remove(temporaryName)
	position := fmt.Sprintf("%.3f", options.Position.Seconds())
	command := exec.CommandContext(ctx, options.Binary, "-hide_banner", "-loglevel", "error", "-nostdin", "-protocol_whitelist", "http,https,tcp,tls,crypto", "-ss", position, "-i", input, "-frames:v", "1", "-an", "-sn", "-f", "image2", "-vcodec", "mjpeg", "-y", temporaryName)
	output, err := command.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("extract cover canceled: %w", ctx.Err())
		}
		message := strings.TrimSpace(string(output))
		if len(message) > 2048 {
			message = message[:2048]
		}
		return fmt.Errorf("ffmpeg failed: %w: %s", err, message)
	}
	info, err := os.Stat(temporaryName)
	if err != nil {
		return fmt.Errorf("ffmpeg did not create output: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("ffmpeg produced an empty or invalid output")
	}
	if err := os.Chmod(temporaryName, 0o644); err != nil {
		return fmt.Errorf("set cover permissions: %w", err)
	}
	if err := os.Rename(temporaryName, cover); err != nil {
		return fmt.Errorf("publish cover: %w", err)
	}
	return nil
}

func ReplaceSTRMContent(root, from, to string, preview bool) (ReplaceResult, error) {
	if from == "" {
		return ReplaceResult{}, fmt.Errorf("search text must not be empty")
	}
	root, err := safeToolRoot(root)
	if err != nil {
		return ReplaceResult{}, err
	}
	result := ReplaceResult{}
	err = filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".strm") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 64<<10 {
			return nil
		}
		result.Scanned++
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		updated := strings.ReplaceAll(string(data), from, to)
		if updated == string(data) {
			return nil
		}
		result.Changed++
		if preview {
			return nil
		}
		return writeAtomic(current, []byte(updated))
	})
	return result, err
}

func ClearGenerated(root string) (int, error) {
	root, err := safeToolRoot(root)
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		target := filepath.Join(root, entry.Name())
		if err := os.RemoveAll(target); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func safeToolRoot(root string) (string, error) {
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) || root == string(filepath.Separator) {
		return "", fmt.Errorf("unsafe task destination %q", root)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("task destination must be a real directory")
	}
	return root, nil
}
