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
	files, err := r.walk(nil)
	if err != nil {
		return nil, err
	}
	r.files, r.listed = files, true
	return files, nil
}

// FilesExcluding is like Files, but also skips directories whose base name is
// in exclude, at any depth. The target directory itself is never excluded.
// Unlike Files it does not use or fill the cache, so note resolution (which
// must still see excluded directories) is unaffected.
func (r *Resolver) FilesExcluding(exclude map[string]bool) ([]string, error) {
	if len(exclude) == 0 {
		return r.Files()
	}
	return r.walk(exclude)
}

func (r *Resolver) walk(exclude map[string]bool) ([]string, error) {
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
			if path != r.Dir && (strings.HasPrefix(entry.Name(), ".") || exclude[entry.Name()]) {
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

// Resolve finds the note named by arg, trying tiers in order and stopping at
// the first tier that matches. A tier with several matches is ambiguous;
// matches in later tiers never override an earlier tier.
//
//  1. A file path (as given, then with .org appended; if arg contains a path
//     separator, also relative to the target directory).
//  2. An exact :ID: property value.
//  3. An exact file name directly in the target directory (.org optional).
//  4. An exact file name anywhere below the target directory.
//  5. A file name prefix, first directly in the target directory, then below it.
func (r *Resolver) Resolve(arg string) (string, error) {
	for _, path := range []string{arg, arg + ".org"} {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
	}
	if strings.ContainsRune(arg, os.PathSeparator) {
		for _, path := range []string{filepath.Join(r.Dir, arg), filepath.Join(r.Dir, arg+".org")} {
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				return path, nil
			}
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
	direct := func(path string) bool {
		return filepath.Dir(r.Relative(path)) == "."
	}
	byName := func(directOnly, nestedOnly bool, match func(name string) bool) []string {
		var matches []string
		for _, path := range files {
			if d := direct(path); (directOnly && !d) || (nestedOnly && d) {
				continue
			}
			if match(filepath.Base(path)) {
				matches = append(matches, path)
			}
		}
		return matches
	}

	// Tier 2: :ID: property, exact value. IDs are properties, independent of
	// heading or block recognition.
	var idMatches []string
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			r.warning(path, err)
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if len(line) >= 4 && strings.EqualFold(line[:4], ":ID:") && strings.TrimSpace(line[4:]) == arg {
				idMatches = append(idMatches, path)
				break
			}
		}
	}
	if len(idMatches) > 0 {
		return choose(idMatches)
	}

	exactName := func(name string) bool { return name == arg || name == arg+".org" }
	prefixName := func(name string) bool {
		return strings.HasPrefix(name, arg) || strings.HasPrefix(strings.TrimSuffix(name, ".org"), arg)
	}
	// Tiers 3-5: exact name in the target directory, exact name anywhere,
	// then prefix in the target directory, then prefix anywhere below it.
	for _, tier := range []struct {
		directOnly, nestedOnly bool
		match                  func(string) bool
	}{
		{true, false, exactName},
		{false, false, exactName},
		{true, false, prefixName},
		{false, true, prefixName},
	} {
		if matches := byName(tier.directOnly, tier.nestedOnly, tier.match); len(matches) > 0 {
			return choose(matches)
		}
	}
	return "", fmt.Errorf("見つかりません: %s", arg)
}
