package tools

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"

	"alga-agent/internal/config"
)

// FileTools provides read_file, write_file, search_files, and patch tools
// scoped to a set of allowed root directories. Ported from hermes-agent's
// file_tools.py, adapted to Go and the TypedTool framework.
type FileTools struct {
	roots       []string
	maxReadSize int64
	patchMu     sync.Mutex
}

// NewFileTools constructs a FileTools from config. Returns nil if disabled or
// when no roots are configured: file tools fail closed and require at least one
// explicitly scoped root before they are registered.
func NewFileTools(cfg config.FileToolsConfig) *FileTools {
	if !cfg.Enabled {
		return nil
	}
	if len(cfg.Roots) == 0 {
		return nil
	}
	abs := make([]string, 0, len(cfg.Roots))
	for _, r := range cfg.Roots {
		a, err := filepath.Abs(r)
		if err != nil {
			continue
		}
		abs = append(abs, resolveSymlinks(a))
	}
	if len(abs) == 0 {
		return nil
	}
	maxRead := cfg.MaxReadBytes
	if maxRead <= 0 {
		maxRead = 256 * 1024
	}
	return &FileTools{roots: abs, maxReadSize: int64(maxRead)}
}

// resolveSymlinks resolves symbolic links in path. When path exists it returns
// filepath.EvalSymlinks(path). When path does not exist (e.g. a file about to
// be created) it resolves the nearest existing ancestor and re-appends the
// non-existent suffix, so the real location is still checked for containment.
func resolveSymlinks(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	dir := path
	suffix := ""
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			return filepath.Join(resolved, suffix)
		}
		suffix = filepath.Join(filepath.Base(dir), suffix)
		dir = parent
	}
	return path
}

func (f *FileTools) allowed(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	resolved := resolveSymlinks(abs)
	for _, root := range f.roots {
		if root == "/" || strings.HasPrefix(resolved, root+string(filepath.Separator)) || resolved == root {
			return nil
		}
	}
	return fmt.Errorf("path %q is outside allowed roots", path)
}

// openVerified opens path with O_NOFOLLOW (rejecting final-component symlinks),
// then verifies the opened file descriptor's real path remains under f.roots.
// This eliminates the TOCTOU race between allowed() and the actual open.
func (f *FileTools) openVerified(path string, flag int, perm os.FileMode) (*os.File, error) {
	file, err := os.OpenFile(path, flag|syscall.O_NOFOLLOW, perm)
	if err != nil {
		return nil, err
	}
	real, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", file.Fd()))
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("resolve opened file: %w", err)
	}
	for _, root := range f.roots {
		if root == "/" || strings.HasPrefix(real, root+string(filepath.Separator)) || real == root {
			return file, nil
		}
	}
	_ = file.Close()
	return nil, fmt.Errorf("opened file %q resolves outside allowed roots", path)
}

// --- read_file ---

type readFileInput struct {
	Path   string `json:"path" desc:"Absolute or relative file path to read."`
	Offset int    `json:"offset,omitempty" desc:"Line number to start from (1-indexed). Default 1."`
	Limit  int    `json:"limit,omitempty" desc:"Maximum lines to read. Default 2000."`
}

type readFileOutput struct {
	Content    string `json:"content"`
	TotalLines int    `json:"total_lines"`
	Offset     int    `json:"offset"`
	Limit      int    `json:"limit"`
	Truncated  bool   `json:"truncated,omitempty"`
}

func (f *FileTools) readFile(ctx context.Context, in readFileInput) Result[readFileOutput] {
	if in.Path == "" {
		return ErrMsg[readFileOutput]("path is required")
	}
	if err := f.allowed(in.Path); err != nil {
		return Err[readFileOutput](err)
	}
	info, err := os.Stat(in.Path)
	if err != nil {
		return Err[readFileOutput](err)
	}
	if info.IsDir() {
		entries, err := os.ReadDir(in.Path)
		if err != nil {
			return Err[readFileOutput](err)
		}
		var sb strings.Builder
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() {
				name += "/"
			}
			sb.WriteString(name + "\n")
		}
		return OK(readFileOutput{Content: sb.String(), TotalLines: len(entries), Offset: 1, Limit: len(entries)})
	}

	file, err := f.openVerified(in.Path, os.O_RDONLY, 0)
	if err != nil {
		return Err[readFileOutput](err)
	}
	defer file.Close()

	offset := in.Offset
	if offset <= 0 {
		offset = 1
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 2000
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	var lines []string
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum < offset {
			continue
		}
		if len(lines) < limit {
			lines = append(lines, fmt.Sprintf("%d: %s", lineNum, scanner.Text()))
		}
	}
	if err := scanner.Err(); err != nil {
		return Err[readFileOutput](err)
	}

	joined := strings.Join(lines, "\n")
	content := joined
	truncated := lineNum >= offset+len(lines)
	if int64(len(content)) > f.maxReadSize {
		content = truncate(content, int(f.maxReadSize))
		truncated = true
	}

	return OK(readFileOutput{
		Content:    content,
		TotalLines: lineNum,
		Offset:     offset,
		Limit:      limit,
		Truncated:  truncated,
	})
}

// --- write_file ---

type writeFileInput struct {
	Path    string `json:"path" desc:"Absolute or relative file path to write."`
	Content string `json:"content" desc:"The full content to write to the file."`
	Append  bool   `json:"append,omitempty" desc:"Append to the file instead of overwriting."`
	Mode    string `json:"mode,omitempty" desc:"File permission mode (e.g. \"0644\"). Default 0644."`
}

type writeFileOutput struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes_written"`
}

func (f *FileTools) writeFile(ctx context.Context, in writeFileInput) Result[writeFileOutput] {
	if in.Path == "" {
		return ErrMsg[writeFileOutput]("path is required")
	}
	if err := f.allowed(in.Path); err != nil {
		return Err[writeFileOutput](err)
	}
	if dir := filepath.Dir(in.Path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Err[writeFileOutput](fmt.Errorf("create directory: %w", err))
		}
	}

	perm := os.FileMode(0o644)
	if in.Mode != "" {
		var p uint32
		if _, err := fmt.Sscanf(in.Mode, "%o", &p); err == nil {
			perm = os.FileMode(p)
		}
	}

	flag := os.O_WRONLY | os.O_CREATE
	if in.Append {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	file, err := f.openVerified(in.Path, flag, perm)
	if err != nil {
		return Err[writeFileOutput](err)
	}

	n, err := file.WriteString(in.Content)
	if err != nil {
		_ = file.Close()
		return Err[writeFileOutput](err)
	}
	if err := file.Close(); err != nil {
		return Err[writeFileOutput](err)
	}
	return OK(writeFileOutput{Path: in.Path, Bytes: n})
}

// --- search_files ---

type searchFilesInput struct {
	Pattern  string `json:"pattern" desc:"Regular expression to search for in file contents."`
	Path     string `json:"path,omitempty" desc:"Directory to search in. Default: current directory."`
	Include  string `json:"include,omitempty" desc:"Glob pattern to filter files (e.g. \"*.go\", \"*.{ts,tsx}\")."`
	MaxDepth int    `json:"max_depth,omitempty" desc:"Maximum directory depth to recurse. Default 10."`
	Limit    int    `json:"limit,omitempty" desc:"Maximum matches to return. Default 50."`
}

type searchMatch struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

type searchFilesOutput struct {
	Matches []searchMatch `json:"matches"`
	Count   int           `json:"count"`
}

func (f *FileTools) searchFiles(ctx context.Context, in searchFilesInput) Result[searchFilesOutput] {
	if in.Pattern == "" {
		return ErrMsg[searchFilesOutput]("pattern is required")
	}
	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return ErrMsg[searchFilesOutput]("invalid regex: " + err.Error())
	}

	root := in.Path
	if root == "" {
		root = "."
	}
	if err := f.allowed(root); err != nil {
		return Err[searchFilesOutput](err)
	}

	maxDepth := in.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 10
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}

	var matches []searchMatch
	rootAbs, _ := filepath.Abs(root)

	if err := ctx.Err(); err != nil {
		return Err[searchFilesOutput](err)
	}

	walkErr := filepath.Walk(rootAbs, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// An unreadable root must surface as a failure, not an empty
			// result; unreadable subdirectories are skipped so a single
			// permission-denied entry does not abort the whole search.
			if path == rootAbs {
				return err
			}
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if len(matches) >= limit {
			return filepath.SkipAll
		}
		rel, _ := filepath.Rel(rootAbs, path)
		depth := strings.Count(rel, string(filepath.Separator))
		if info.IsDir() {
			if depth >= maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > 5*1024*1024 {
			return nil
		}
		if in.Include != "" && !matchGlob(in.Include, filepath.Base(path)) {
			return nil
		}
		if isBinaryFile(path) {
			return nil
		}

		file, err := f.openVerified(path, os.O_RDONLY, 0)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)
		lineNum := 0
		for scanner.Scan() && len(matches) < limit {
			lineNum++
			line := scanner.Text()
			if re.MatchString(line) {
				trimmed := line
				if len(trimmed) > 200 {
					trimmed = trimmed[:200] + "..."
				}
				matches = append(matches, searchMatch{
					File:    path,
					Line:    lineNum,
					Content: strings.TrimSpace(trimmed),
				})
			}
		}
		return nil
	})
	if walkErr != nil {
		return Err[searchFilesOutput](fmt.Errorf("search traversal: %w", walkErr))
	}

	return OK(searchFilesOutput{Matches: matches, Count: len(matches)})
}

// --- patch ---

type patchInput struct {
	Path       string `json:"path" desc:"File path to patch."`
	OldString  string `json:"old_string" desc:"The exact text to find and replace."`
	NewString  string `json:"new_string" desc:"The replacement text."`
	ReplaceAll bool   `json:"replace_all,omitempty" desc:"Replace all occurrences instead of just the first."`
}

type patchOutput struct {
	Path         string `json:"path"`
	Replacements int    `json:"replacements"`
}

func (f *FileTools) patch(ctx context.Context, in patchInput) Result[patchOutput] {
	if in.Path == "" {
		return ErrMsg[patchOutput]("path is required")
	}
	if in.OldString == "" {
		return ErrMsg[patchOutput]("old_string is required")
	}
	if in.OldString == in.NewString {
		return ErrMsg[patchOutput]("old_string and new_string must differ")
	}
	if err := f.allowed(in.Path); err != nil {
		return Err[patchOutput](err)
	}

	f.patchMu.Lock()
	defer f.patchMu.Unlock()

	pf, err := f.openVerified(in.Path, os.O_RDONLY, 0)
	if err != nil {
		return Err[patchOutput](err)
	}
	data, err := io.ReadAll(pf)
	_ = pf.Close()
	if err != nil {
		return Err[patchOutput](err)
	}
	content := string(data)

	count := strings.Count(content, in.OldString)
	if count == 0 {
		return ErrMsg[patchOutput]("old_string not found in file")
	}

	var result string
	if in.ReplaceAll {
		result = strings.ReplaceAll(content, in.OldString, in.NewString)
	} else {
		if count > 1 {
			return ErrMsg[patchOutput](fmt.Sprintf("old_string found %d times; provide more context or set replace_all", count))
		}
		result = strings.Replace(content, in.OldString, in.NewString, 1)
	}

	info, _ := os.Stat(in.Path)
	perm := os.FileMode(0o644)
	if info != nil {
		perm = info.Mode().Perm()
	}

	// Write to a temp file in the target's directory and rename over the
	// original so the replacement is atomic and a failure never leaves a
	// partially written file.
	tmp, err := os.CreateTemp(filepath.Dir(in.Path), ".patch-*")
	if err != nil {
		return Err[patchOutput](fmt.Errorf("create temp file: %w", err))
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.WriteString(result); err != nil {
		_ = tmp.Close()
		return Err[patchOutput](err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return Err[patchOutput](err)
	}
	if err := tmp.Close(); err != nil {
		return Err[patchOutput](err)
	}
	if err := os.Rename(tmpPath, in.Path); err != nil {
		return Err[patchOutput](err)
	}

	replacements := 1
	if in.ReplaceAll {
		replacements = count
	}
	return OK(patchOutput{Path: in.Path, Replacements: replacements})
}

// RegisterFileTools registers all file tools against the registry.
func RegisterFileTools(reg *Registry, ft *FileTools) {
	if ft == nil {
		return
	}
	reg.Register(NewTypedTool("read_file",
		"Read a file or directory listing. Returns numbered lines. Use offset/limit for large files.",
		ft.readFile, WithCategory[readFileInput, readFileOutput]("System")))
	reg.Register(NewTypedTool("write_file",
		"Write content to a file. Creates parent directories. Use append mode to add to existing files.",
		ft.writeFile, WithCategory[writeFileInput, writeFileOutput]("System")))
	reg.Register(NewTypedTool("search_files",
		"Search file contents using a regular expression. Returns matching lines with file paths and line numbers.",
		ft.searchFiles, WithCategory[searchFilesInput, searchFilesOutput]("System")))
	reg.Register(NewTypedTool("patch",
		"Replace exact text in a file. Fails if old_string is not found or matches multiple times (unless replace_all).",
		ft.patch, WithCategory[patchInput, patchOutput]("System")))
}

func matchGlob(pattern, name string) bool {
	for _, braced := range expandBraces(pattern) {
		for _, raw := range strings.Split(braced, ",") {
			p := strings.TrimSpace(raw)
			if p == "" {
				continue
			}
			trimmed := strings.TrimPrefix(p, "*.")
			if matched, _ := filepath.Match(trimmed, name); matched {
				return true
			}
			if matched, _ := filepath.Match("*."+trimmed, name); matched {
				return true
			}
		}
	}
	if matched, _ := filepath.Match(pattern, name); matched {
		return true
	}
	return false
}

// expandBraces expands brace groups such as "*.{ts,tsx}" into one pattern per
// alternative ("*.ts", "*.tsx"). Patterns without braces are returned unchanged.
func expandBraces(pattern string) []string {
	open := strings.Index(pattern, "{")
	if open < 0 {
		return []string{pattern}
	}
	rest := strings.Index(pattern[open:], "}")
	if rest < 0 {
		return []string{pattern}
	}
	close := open + rest
	prefix := pattern[:open]
	suffix := pattern[close+1:]
	var out []string
	for _, alt := range strings.Split(pattern[open+1:close], ",") {
		out = append(out, expandBraces(prefix+alt+suffix)...)
	}
	return out
}

var binaryExtensions = map[string]struct{}{
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".bmp": {}, ".ico": {},
	".pdf": {}, ".zip": {}, ".tar": {}, ".gz": {}, ".bz2": {}, ".xz": {},
	".so": {}, ".o": {}, ".a": {}, ".bin": {}, ".exe": {}, ".dll": {},
	".woff": {}, ".woff2": {}, ".ttf": {}, ".eot": {}, ".mp3": {}, ".mp4": {},
	".avi": {}, ".mov": {}, ".wav": {}, ".class": {}, ".jar": {}, ".pyc": {},
}

func isBinaryFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if _, ok := binaryExtensions[ext]; ok {
		return true
	}
	file, err := os.Open(path)
	if err != nil {
		return true
	}
	defer file.Close()
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil || n == 0 {
		return true
	}
	for _, b := range buf[:n] {
		if b == 0 {
			return true
		}
	}
	return false
}
