package orgread

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Resolver struct {
	Dir    string
	Warn   io.Writer
	files  []string
	listed bool
}

// Files lists .org files in deterministic path order, skipping hidden directories.
// A failure to read the root is fatal; inaccessible descendants are warned about.
func (r *Resolver) Files() ([]string, error) {
	if r.listed {
		return r.files, nil
	}
	var files []string
	err := filepath.WalkDir(r.Dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == r.Dir {
				return err
			}
			r.warning(path, err)
			return nil
		}
		if entry.IsDir() {
			if path != r.Dir && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".org") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	r.files, r.listed = files, true
	return files, nil
}

func (r *Resolver) warning(path string, err error) {
	if r.Warn != nil {
		fmt.Fprintf(r.Warn, "警告: %s: %v\n", path, err)
	}
}

func (r *Resolver) Relative(path string) string {
	if rel, err := filepath.Rel(r.Dir, path); err == nil {
		return rel
	}
	return path
}

type AmbiguousNoteError struct {
	Note       string
	Candidates []string
}

func (e *AmbiguousNoteError) Error() string {
	return "ノートが曖昧です: " + e.Note + "\n" + strings.Join(e.Candidates, "\n")
}

func (r *Resolver) Resolve(arg string) (string, error) {
	for _, path := range []string{arg, arg + ".org"} {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
	}
	files, err := r.Files()
	if err != nil {
		return "", err
	}
	choose := func(matches []string) (string, error) {
		if len(matches) == 1 {
			return matches[0], nil
		}
		candidates := make([]string, len(matches))
		for i, path := range matches {
			candidates[i] = r.Relative(path)
		}
		return "", &AmbiguousNoteError{Note: arg, Candidates: candidates}
	}
	for _, exact := range []bool{true, false} {
		var matches []string
		for _, path := range files {
			name := filepath.Base(path)
			match := name == arg || name == arg+".org"
			if !exact {
				match = strings.HasPrefix(name, arg) || strings.HasPrefix(strings.TrimSuffix(name, ".org"), arg)
			}
			if match {
				matches = append(matches, path)
			}
		}
		if len(matches) > 0 {
			return choose(matches)
		}
	}
	var matches []string
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			r.warning(path, err)
			continue
		}
		// IDs are properties, independent of heading or block recognition.
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if len(line) >= 4 && strings.EqualFold(line[:4], ":ID:") && strings.TrimSpace(line[4:]) == arg {
				matches = append(matches, path)
				break
			}
		}
	}
	if len(matches) > 0 {
		return choose(matches)
	}
	return "", fmt.Errorf("見つかりません: %s", arg)
}
