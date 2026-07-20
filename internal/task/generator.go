package task

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/signature"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/storage"
)

type Result struct {
	Scanned int `json:"scanned"`
	Created int `json:"created"`
	Copied  int `json:"copied"`
	Removed int `json:"removed"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

const maximumEntryFailureDetails = 20
const maximumEntryFailureDetailBytes = 512

type entryFailures struct {
	details []string
	total   int
}

func (f *entryFailures) add(result *Result, err error) {
	f.total++
	result.Failed++
	if len(f.details) < maximumEntryFailureDetails {
		detail := err.Error()
		if len(detail) > maximumEntryFailureDetailBytes {
			detail = truncateUTF8Bytes(detail, maximumEntryFailureDetailBytes-3) + "..."
		}
		f.details = append(f.details, detail)
	}
}

func (f *entryFailures) err() error {
	if f.total == 0 {
		return nil
	}
	parts := make([]string, 0, len(f.details)+1)
	parts = append(parts, f.details...)
	if omitted := f.total - len(f.details); omitted > 0 {
		parts = append(parts, fmt.Sprintf("%d additional errors omitted", omitted))
	}
	return fmt.Errorf("%d entries failed: %s", f.total, strings.Join(parts, "; "))
}

type directoryListError struct{ err error }

func (e *directoryListError) Error() string { return e.err.Error() }
func (e *directoryListError) Unwrap() error { return e.err }

type Preview struct {
	Source  string `json:"source"`
	Target  string `json:"target"`
	Content string `json:"content"`
}

type Generator struct {
	publicURL string
	token     string
	plugins   []config.PluginConfig
}

func NewGenerator(publicURL, token string, plugins ...config.PluginConfig) *Generator {
	return &Generator{publicURL: strings.TrimRight(publicURL, "/"), token: token, plugins: append([]config.PluginConfig(nil), plugins...)}
}

func (g *Generator) Run(ctx context.Context, cfg config.TaskConfig, source storage.Storage, overridePath string) (Result, error) {
	cfg.Plugins = append(append([]config.PluginConfig(nil), g.plugins...), cfg.Plugins...)
	root := storage.CleanRemote(cfg.Source)
	current := root
	if overridePath != "" {
		current = storage.CleanRemote(overridePath)
	}
	if current != root && !strings.HasPrefix(current, strings.TrimRight(root, "/")+"/") {
		return Result{}, fmt.Errorf("override path %q is outside task source %q", current, root)
	}
	result := Result{}
	expected := make(map[string]struct{})
	failures := &entryFailures{}
	err := g.walk(ctx, cfg, source, root, current, expected, &result, failures)
	if err != nil {
		return result, err
	}
	if err := failures.err(); err != nil {
		return result, err
	}
	if cfg.SyncDelete && overridePath == "" {
		removed, err := removeStale(cfg.Destination, expected, cfg.KeepLocal, cfg.CopyExt)
		result.Removed = removed
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func (g *Generator) Preview(ctx context.Context, cfg config.TaskConfig, source storage.Storage, sourcePath string) (Preview, error) {
	cfg.Plugins = append(append([]config.PluginConfig(nil), g.plugins...), cfg.Plugins...)
	root := storage.CleanRemote(cfg.Source)
	remotePath := storage.CleanRemote(sourcePath)
	if remotePath != root && !strings.HasPrefix(remotePath, strings.TrimRight(root, "/")+"/") {
		return Preview{}, fmt.Errorf("preview path %q is outside task source %q", remotePath, root)
	}
	entries, err := source.List(ctx, path.Dir(remotePath))
	if err != nil {
		return Preview{}, fmt.Errorf("list preview parent for %q: %w", remotePath, err)
	}
	if err := preflightDirectoryTargets(cfg, root, entries, false); err != nil {
		return Preview{}, err
	}
	var entry storage.Entry
	found := false
	for _, candidate := range entries {
		if storage.CleanRemote(candidate.Path) == remotePath {
			entry, found = candidate, true
			break
		}
	}
	if !found {
		return Preview{}, fmt.Errorf("preview source file %q was not found", remotePath)
	}
	if entry.IsDir {
		return Preview{}, fmt.Errorf("preview source %q is a directory", remotePath)
	}
	if entry.Size < cfg.MinSize || (cfg.MaxSize > 0 && entry.Size > cfg.MaxSize) {
		return Preview{}, fmt.Errorf("preview source %q is outside the task size limits", remotePath)
	}
	originalPath := entry.Path
	previewParent, err := previewParentPath(root, remotePath, cfg.Plugins)
	if err != nil {
		return Preview{}, err
	}
	entry.Path = path.Join(previewParent, entry.Name)
	name, skip, err := applyPlugins(entry.Name, entry.IsDir, cfg.Plugins)
	if err != nil {
		return Preview{}, err
	}
	if skip {
		return Preview{}, fmt.Errorf("preview source %q is skipped by task plugins", remotePath)
	}
	if sanitized, changed := illegalFilename(name, cfg.Plugins); changed {
		entry.Path = path.Join(path.Dir(entry.Path), sanitized)
		entry.Name = sanitized
		name = sanitized
	}
	target, isMedia, err := generatedSTRMTarget(cfg, root, entry, name)
	if err != nil {
		return Preview{}, err
	}
	if !isMedia {
		return Preview{}, fmt.Errorf("preview source %q is not a configured media file", remotePath)
	}
	content, err := g.streamContent(ctx, cfg, source, entry.Path, originalPath)
	if err != nil {
		return Preview{}, err
	}
	relativeTarget, err := filepath.Rel(cfg.Destination, target)
	if err != nil || relativeTarget == "." || relativeTarget == ".." || strings.HasPrefix(relativeTarget, ".."+string(filepath.Separator)) {
		return Preview{}, fmt.Errorf("preview target is outside the task destination")
	}
	return Preview{Source: remotePath, Target: filepath.ToSlash(relativeTarget), Content: content + "\n"}, nil
}

func previewParentPath(root, remotePath string, plugins []config.PluginConfig) (string, error) {
	parent := path.Dir(remotePath)
	relative := strings.TrimPrefix(strings.TrimPrefix(parent, root), "/")
	transformed := root
	if relative == "" || relative == "." {
		return transformed, nil
	}
	for _, component := range strings.Split(relative, "/") {
		name, skip, err := applyPlugins(component, true, plugins)
		if err != nil {
			return "", err
		}
		if skip {
			return "", fmt.Errorf("preview source %q is inside a directory skipped by task plugins", remotePath)
		}
		if sanitized, changed := illegalFilename(name, plugins); changed {
			name = sanitized
		}
		transformed = path.Join(transformed, name)
	}
	return transformed, nil
}

func (g *Generator) walk(ctx context.Context, cfg config.TaskConfig, source storage.Storage, taskRoot, current string, expected map[string]struct{}, result *Result, failures *entryFailures) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := source.List(ctx, current)
	if err != nil {
		return &directoryListError{err: fmt.Errorf("list %s: %w", current, err)}
	}
	if err := pluginDelay(ctx, cfg.Plugins); err != nil {
		return err
	}
	_, copiesLocal := source.(*storage.Local)
	if err := preflightDirectoryTargets(cfg, taskRoot, entries, copiesLocal); err != nil {
		return err
	}
	mediaNames, err := directoryMediaNames(cfg, entries)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		result.Scanned++
		name, skip, err := applyPlugins(entry.Name, entry.IsDir, cfg.Plugins)
		if err != nil {
			return err
		}
		if skip {
			result.Skipped++
			continue
		}
		if sanitized, changed := illegalFilename(name, cfg.Plugins); changed {
			if err := source.Rename(ctx, entry.Path, sanitized); err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				failures.add(result, fmt.Errorf("sanitize remote filename %q: %w", entry.Path, err))
				continue
			}
			entry.Path = path.Join(path.Dir(entry.Path), sanitized)
			entry.Name = sanitized
			name = sanitized
		}
		if entry.IsDir {
			localDirectory := localDestinationDirectory(cfg.Destination, taskRoot, entry.Path)
			if cfg.DirTimeCheck && !entry.ModTime.IsZero() {
				if info, statErr := os.Stat(localDirectory); statErr == nil && !info.ModTime().Before(entry.ModTime) {
					if err := markExisting(localDirectory, expected); err != nil {
						failures.add(result, fmt.Errorf("mark existing directory %q: %w", entry.Path, err))
						continue
					}
					result.Skipped++
					continue
				}
			}
			if err := g.walk(ctx, cfg, source, taskRoot, entry.Path, expected, result, failures); err != nil {
				var listErr *directoryListError
				if errors.As(err, &listErr) && ctx.Err() == nil {
					failures.add(result, listErr)
					continue
				}
				return err
			}
			if cfg.DirTimeCheck && !entry.ModTime.IsZero() {
				if err := os.MkdirAll(localDirectory, 0o755); err != nil {
					failures.add(result, fmt.Errorf("create destination directory for %q: %w", entry.Path, err))
					continue
				}
				if err := os.Chtimes(localDirectory, entry.ModTime, entry.ModTime); err != nil {
					failures.add(result, fmt.Errorf("set destination directory time for %q: %w", entry.Path, err))
					continue
				}
			}
			continue
		}
		if entry.Size < cfg.MinSize || (cfg.MaxSize > 0 && entry.Size > cfg.MaxSize) {
			result.Skipped++
			continue
		}
		if name == "." || name == ".." || path.Base(name) != name || strings.Contains(name, `\`) {
			return fmt.Errorf("plugin produced unsafe filename %q", name)
		}
		ext := strings.ToLower(path.Ext(name))
		relative := strings.TrimPrefix(strings.TrimPrefix(entry.Path, taskRoot), "/")
		relative = path.Join(path.Dir(relative), name)
		if contains(cfg.CopyExt, ext) {
			localPath, ok := source.(*storage.Local)
			if !ok {
				result.Skipped++
				continue
			}
			sourceFile, err := localPath.FilePath(entry.Path)
			if err != nil {
				failures.add(result, fmt.Errorf("resolve local copy source %q: %w", entry.Path, err))
				continue
			}
			relative = path.Join(path.Dir(relative), copiedAssetName(name, mediaNames))
			target := filepath.Join(cfg.Destination, filepath.FromSlash(relative))
			if err := copyFile(sourceFile, target); err != nil {
				failures.add(result, fmt.Errorf("copy asset %q: %w", entry.Path, err))
				continue
			}
			if err := pluginDelay(ctx, cfg.Plugins); err != nil {
				return err
			}
			expected[filepath.Clean(target)] = struct{}{}
			result.Copied++
			continue
		}
		if !contains(defaultMedia(cfg.MediaExt), ext) {
			result.Skipped++
			continue
		}
		target, isMedia, err := generatedSTRMTarget(cfg, taskRoot, entry, name)
		if err != nil {
			return err
		}
		if !isMedia {
			return fmt.Errorf("internal media target mismatch for %q", entry.Path)
		}
		if cfg.Incremental {
			if _, err := os.Stat(target); err == nil {
				expected[filepath.Clean(target)] = struct{}{}
				result.Skipped++
				continue
			}
		}
		content, err := g.streamContent(ctx, cfg, source, entry.Path, entry.Path)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			failures.add(result, fmt.Errorf("build STRM content for %q: %w", entry.Path, err))
			continue
		}
		if err := writeAtomic(target, []byte(content+"\n")); err != nil {
			failures.add(result, fmt.Errorf("write STRM for %q: %w", entry.Path, err))
			continue
		}
		expected[filepath.Clean(target)] = struct{}{}
		result.Created++
	}
	return nil
}

func preflightDirectoryTargets(cfg config.TaskConfig, taskRoot string, entries []storage.Entry, copiesLocal bool) error {
	targets := make(map[string]string)
	mediaNames, err := directoryMediaNames(cfg, entries)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir || entry.Size < cfg.MinSize || (cfg.MaxSize > 0 && entry.Size > cfg.MaxSize) {
			continue
		}
		name, skip, err := applyPlugins(entry.Name, entry.IsDir, cfg.Plugins)
		if err != nil {
			return err
		}
		if skip {
			continue
		}
		if sanitized, changed := illegalFilename(name, cfg.Plugins); changed {
			name = sanitized
		}
		ext := strings.ToLower(path.Ext(name))
		var target string
		var selected bool
		if copiesLocal && contains(cfg.CopyExt, ext) {
			relative := strings.TrimPrefix(strings.TrimPrefix(entry.Path, taskRoot), "/")
			relative = path.Join(path.Dir(relative), copiedAssetName(name, mediaNames))
			target = filepath.Join(cfg.Destination, filepath.FromSlash(relative))
			selected = true
		} else {
			var isMedia bool
			target, isMedia, err = generatedSTRMTarget(cfg, taskRoot, entry, name)
			selected = isMedia
		}
		if err != nil {
			return err
		}
		if !selected {
			continue
		}
		cleaned := filepath.Clean(target)
		if previous, exists := targets[cleaned]; exists {
			return fmt.Errorf("STRM filename collision: %q and %q both map to %q", previous, entry.Path, cleaned)
		}
		targets[cleaned] = entry.Path
	}
	return nil
}

func directoryMediaNames(cfg config.TaskConfig, entries []storage.Entry) ([]string, error) {
	names := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir || entry.Size < cfg.MinSize || (cfg.MaxSize > 0 && entry.Size > cfg.MaxSize) {
			continue
		}
		name, skip, err := applyPlugins(entry.Name, false, cfg.Plugins)
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}
		if sanitized, changed := illegalFilename(name, cfg.Plugins); changed {
			name = sanitized
		}
		if contains(defaultMedia(cfg.MediaExt), strings.ToLower(path.Ext(name))) {
			names = append(names, name)
		}
	}
	return names, nil
}

func copiedAssetName(assetName string, mediaNames []string) string {
	for _, mediaName := range mediaNames {
		if filenamePrefix(assetName, mediaName) {
			return assetName
		}
	}
	assetStem := strings.TrimSuffix(assetName, path.Ext(assetName))
	longest := -1
	candidates := make(map[string]struct{})
	for _, mediaName := range mediaNames {
		mediaStem := strings.TrimSuffix(mediaName, path.Ext(mediaName))
		if !filenamePrefix(assetStem, mediaStem) {
			continue
		}
		if len(mediaStem) > longest {
			longest = len(mediaStem)
			clear(candidates)
		}
		if len(mediaStem) == longest {
			candidates[assetName[:len(mediaStem)]+path.Ext(mediaName)+assetName[len(mediaStem):]] = struct{}{}
		}
	}
	if len(candidates) != 1 {
		return assetName
	}
	for candidate := range candidates {
		return candidate
	}
	return assetName
}

func filenamePrefix(value, prefix string) bool {
	if len(value) < len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return false
	}
	return len(value) == len(prefix) || strings.ContainsRune(".-_", rune(value[len(prefix)]))
}

func generatedSTRMTarget(cfg config.TaskConfig, taskRoot string, entry storage.Entry, name string) (string, bool, error) {
	if name == "." || name == ".." || path.Base(name) != name || strings.Contains(name, `\`) {
		err := fmt.Errorf("plugin produced unsafe filename %q", name)
		return "", false, err
	}
	ext := strings.ToLower(path.Ext(name))
	if contains(cfg.CopyExt, ext) || !contains(defaultMedia(cfg.MediaExt), ext) {
		return "", false, nil
	}
	relative := strings.TrimPrefix(strings.TrimPrefix(entry.Path, taskRoot), "/")
	relative = path.Join(path.Dir(relative), name)
	targetName, err := strmFilename(relative, ext, cfg.Plugins)
	if err != nil {
		wrapped := fmt.Errorf("build STRM filename for %q: %w", entry.Path, err)
		return "", false, wrapped
	}
	target := filepath.Join(cfg.Destination, filepath.FromSlash(targetName))
	return target, true, nil
}

func localDestinationDirectory(destination, taskRoot, remotePath string) string {
	relative := strings.TrimPrefix(strings.TrimPrefix(storage.CleanRemote(remotePath), storage.CleanRemote(taskRoot)), "/")
	return filepath.Join(destination, filepath.FromSlash(relative))
}

func markExisting(root string, expected map[string]struct{}) error {
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("visit %q: %w", current, err)
		}
		if !entry.IsDir() {
			expected[filepath.Clean(current)] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("mark existing files under %q: %w", root, err)
	}
	return nil
}

func (g *Generator) streamURL(storageID, remotePath string) string {
	signed := signature.Create(g.token, storageID, remotePath)
	return g.publicURL + "/stream/" + url.PathEscape(storageID) + "?path=" + url.QueryEscape(remotePath) + "&sig=" + url.QueryEscape(signed)
}

func (g *Generator) fileIDStreamURL(storageID, fileID string) string {
	signed := signature.Create(g.token, storageID, "id:"+fileID)
	return g.publicURL + "/stream/" + url.PathEscape(storageID) + "?id=" + url.QueryEscape(fileID) + "&sig=" + url.QueryEscape(signed)
}

func (g *Generator) streamContent(ctx context.Context, cfg config.TaskConfig, source storage.Storage, streamPath, fileIDPath string) (string, error) {
	streamURL := g.streamURL(cfg.StorageID, streamPath)
	if cfg.FileIDMode {
		fileIDSource, ok := source.(storage.FileIDStorage)
		if !ok {
			return "", fmt.Errorf("storage %q does not support stable file IDs", cfg.StorageID)
		}
		fileID, err := fileIDSource.FileID(ctx, fileIDPath)
		if err != nil {
			return "", fmt.Errorf("resolve stable file ID for %q: %w", fileIDPath, err)
		}
		streamURL = g.fileIDStreamURL(cfg.StorageID, fileID)
	}
	return applySTRMPlugins(streamURL, cfg.Plugins)
}

func applyPlugins(name string, isDir bool, plugins []config.PluginConfig) (string, bool, error) {
	result := name
	for _, plugin := range plugins {
		switch plugin.Type {
		case "skip_regex", "replace_regex", "filename":
			re, err := regexp.Compile(plugin.Pattern)
			if err != nil {
				return "", false, fmt.Errorf("plugin %s: %w", plugin.Type, err)
			}
			if plugin.Type == "skip_regex" {
				if re.MatchString(result) {
					return result, true, nil
				}
			} else {
				result = re.ReplaceAllString(result, plugin.Replacement)
			}
		case "filename_skip":
			if plugin.DirectoryOnly && !isDir {
				continue
			}
			matched, err := filenameSkipMatches(result, plugin)
			if err != nil {
				return "", false, err
			}
			include := plugin.FilterMode == "include"
			if matched != include {
				return result, true, nil
			}
		case "strm_replace", "illegal_filename", "custom_strm_filename", "infuse_iso", "request_delay":
			// Applied at the matching generation phase.
		default:
			return "", false, fmt.Errorf("unsupported plugin %q", plugin.Type)
		}
	}
	return result, false, nil
}

func filenameSkipMatches(name string, plugin config.PluginConfig) (bool, error) {
	if err := config.ValidatePlugin(plugin); err != nil {
		return false, fmt.Errorf("plugin %w", err)
	}
	pattern, value := plugin.Pattern, name
	if plugin.MatchMode == "regex" {
		if !plugin.CaseSensitive {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, fmt.Errorf("plugin filename_skip: %w", err)
		}
		return re.MatchString(value), nil
	}
	if !plugin.CaseSensitive {
		pattern, value = strings.ToLower(pattern), strings.ToLower(value)
	}
	return strings.Contains(value, pattern), nil
}

func applySTRMPlugins(content string, plugins []config.PluginConfig) (string, error) {
	result := content
	for _, plugin := range plugins {
		if plugin.Type != "strm_replace" {
			continue
		}
		if plugin.Pattern == "" {
			return "", fmt.Errorf("strm_replace pattern must not be empty")
		}
		re, err := regexp.Compile(plugin.Pattern)
		if err != nil {
			return "", fmt.Errorf("strm_replace: %w", err)
		}
		result = re.ReplaceAllString(result, plugin.Replacement)
	}
	return result, nil
}

func illegalFilename(name string, plugins []config.PluginConfig) (string, bool) {
	enabled, maxBytes := false, 240
	for _, plugin := range plugins {
		if plugin.Type == "illegal_filename" {
			enabled = true
			if plugin.MaxBytes > 0 {
				maxBytes = plugin.MaxBytes
			}
		}
	}
	if !enabled {
		return name, false
	}
	cleanedName := strings.TrimSpace(name)
	re := regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	cleanedName = re.ReplaceAllString(cleanedName, "_")
	ext := path.Ext(cleanedName)
	base := strings.TrimSpace(strings.TrimSuffix(cleanedName, ext))
	if base == "" {
		base = "_"
	}
	if len([]byte(ext)) < maxBytes {
		base = truncateUTF8Bytes(base, maxBytes-len([]byte(ext)))
		if base == "" {
			base = "_"
		}
		result := base + ext
		return result, result != name
	}
	result := truncateUTF8Bytes(base+ext, maxBytes)
	if result == "" {
		result = "_"
	}
	return result, result != name
}

func truncateUTF8Bytes(value string, maximum int) string {
	for len([]byte(value)) > maximum && value != "" {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	return value
}

func strmFilename(relative, mediaExt string, plugins []config.PluginConfig) (string, error) {
	directory, base := path.Dir(relative), path.Base(relative)
	name := strings.TrimSuffix(base, path.Ext(base)) + ".strm"
	for _, plugin := range plugins {
		switch plugin.Type {
		case "infuse_iso":
			if strings.EqualFold(mediaExt, ".iso") {
				name = base + ".strm"
			}
		case "custom_strm_filename":
			if plugin.Template == "" {
				err := fmt.Errorf("custom_strm_filename template is required")
				return "", err
			}
			name = strings.NewReplacer("{name}", strings.TrimSuffix(base, path.Ext(base)), "{filename}", base, "{ext}", mediaExt).Replace(plugin.Template)
			if err := storage.ValidateName(name); err != nil {
				wrapped := fmt.Errorf("validate custom STRM filename: %w", err)
				return "", wrapped
			}
		}
	}
	result := path.Join(directory, name)
	return result, nil
}

func pluginDelay(ctx context.Context, plugins []config.PluginConfig) error {
	delay := 0
	for _, plugin := range plugins {
		if plugin.Type == "request_delay" && plugin.DelayMS > delay {
			delay = plugin.DelayMS
		}
	}
	if delay <= 0 {
		return nil
	}
	if delay > 60000 {
		return fmt.Errorf("request_delay must not exceed 60000ms")
	}
	timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func defaultMedia(values []string) []string {
	if len(values) > 0 {
		return values
	}
	return []string{".mp4", ".mkv", ".avi", ".mov", ".iso", ".ts", ".m2ts", ".flv", ".wmv", ".mp3", ".flac"}
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if strings.EqualFold(strings.TrimSpace(item), value) {
			return true
		}
	}
	return false
}

func writeAtomic(target string, data []byte) error {
	err := atomicFile(target, func(writer io.Writer) error {
		if _, err := writer.Write(data); err != nil {
			return fmt.Errorf("write data: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("write target %q: %w", target, err)
	}
	return nil
}

func copyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = input.Close() }()
	err = atomicFile(target, func(writer io.Writer) error {
		if _, err := io.Copy(writer, input); err != nil {
			return fmt.Errorf("copy source: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("copy to target %q: %w", target, err)
	}
	return nil
}

func atomicFile(target string, write func(io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".smartstrm-copy-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := write(temporary); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set target permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(temporaryName, target); err != nil {
		return fmt.Errorf("replace target file: %w", err)
	}
	return nil
}

func removeStale(root string, expected map[string]struct{}, keep, copiedExtensions []string) (int, error) {
	removed := 0
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if _, exists := expected[filepath.Clean(current)]; exists {
			return nil
		}
		if ext := strings.ToLower(filepath.Ext(entry.Name())); ext != ".strm" && !contains(copiedExtensions, ext) {
			return nil
		}
		for _, pattern := range keep {
			if matched, _ := filepath.Match(pattern, entry.Name()); matched {
				return nil
			}
		}
		if err := os.Remove(current); err != nil {
			return err
		}
		removed++
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	return removed, err
}
