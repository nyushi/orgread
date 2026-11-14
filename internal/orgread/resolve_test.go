package orgread

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "notes")
	for _, tc := range []struct {
		name, arg, want string
		ambiguous       []string
		missing         bool
	}{
		{name: "direct path", arg: filepath.Join(dir, "guide.org"), want: "guide.org"},
		{name: "path without extension", arg: filepath.Join(dir, "guide"), want: "guide.org"},
		{name: "exact name", arg: "guide.org", want: "guide.org"},
		{name: "omitted extension", arg: "guide", want: "guide.org"},
		{name: "recursive exact", arg: "plain", want: "nested/plain.org"},
		{name: "prefix", arg: "20261018161214-a", want: "20261018161214-alpha.org"},
		{name: "property ID", arg: "fictional-guide-id", want: "guide.org"},
		{name: "prefix prefers direct", arg: "20261018161214", want: "20261018161214-alpha.org"},
		{name: "missing", arg: "nonexistent", missing: true},
		{name: "hidden skipped", arg: "secret", missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &Resolver{Dir: dir}
			path, err := r.Resolve(tc.arg)
			if tc.missing {
				if err == nil || !strings.Contains(err.Error(), "見つかりません: "+tc.arg) {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if tc.ambiguous != nil {
				var ambiguity *AmbiguousNoteError
				if !errors.As(err, &ambiguity) || !reflect.DeepEqual(ambiguity.Candidates, tc.ambiguous) {
					t.Fatalf("ambiguity = %#v, error = %v", ambiguity, err)
				}
				return
			}
			if err != nil || r.Relative(path) != tc.want {
				t.Fatalf("path = %q, error = %v, want %q", path, err, tc.want)
			}
		})
	}
}

func TestResolutionPrecedenceAndAmbiguity(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{
		"alpha.org":           "* Root alpha\n",
		"sub/alpha.org":       "* Sub alpha\n",
		"beta.org":            "* File beta\n",
		"holder.org":          "* Holder\n:ID: beta\n",
		"mixed.org":           "* Mixed\n  :iD: Mixed-ID  \n",
		"sub1/gamma.org":      "* Gamma 1\n",
		"sub2/gamma.org":      "* Gamma 2\n",
		"delta.org":           "* Root delta\n",
		"sub/delta-extra.org": "* Sub delta extra\n",
		"sub/epsilon-1.org":   "* Epsilon 1\n",
		"sub2/epsilon-2.org":  "* Epsilon 2\n",
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		arg, want string
		ambiguous bool
	}{
		{"beta", "holder.org", false},             // :ID: tier beats file name tiers
		{"Mixed-ID", "mixed.org", false},          // :ID: marker is case-insensitive
		{"alpha", "alpha.org", false},             // direct exact beats nested exact
		{"gamma", "", true},                       // same tier: two nested exact names
		{"delt", "delta.org", false},              // prefix: direct tier first
		{"delta-e", "sub/delta-extra.org", false}, // prefix: falls through to nested tier
		{"epsilon", "", true},                     // same tier: two nested prefixes
	} {
		t.Run(tc.arg, func(t *testing.T) {
			r := &Resolver{Dir: dir}
			path, err := r.Resolve(tc.arg)
			var ambiguity *AmbiguousNoteError
			if tc.ambiguous {
				if !errors.As(err, &ambiguity) || len(ambiguity.Candidates) != 2 {
					t.Fatalf("expected two candidates, got %v", err)
				}
			} else if err != nil || r.Relative(path) != tc.want {
				t.Fatalf("path = %q, error = %v, want %q", path, err, tc.want)
			}
		})
	}
	// An explicit path wins even when the target directory cannot be read.
	r := &Resolver{Dir: filepath.Join(dir, "missing")}
	if _, err := r.Resolve(filepath.Join(dir, "alpha.org")); err != nil {
		t.Fatal(err)
	}
}

// The layout from the fix request: the same file name directly in the target
// directory and in now/, where only the direct file has an :ID:.
func TestResolvePriorityFixtures(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "priority")
	for _, tc := range []struct {
		name, arg, want string
		ambiguous       []string
	}{
		{name: "file name picks direct", arg: "same", want: "same.org"},
		{name: "file name with extension picks direct", arg: "same.org", want: "same.org"},
		{name: "ID picks direct", arg: "priority-same-id", want: "same.org"},
		{name: "prefix picks direct", arg: "sam", want: "same.org"},
		{name: "nested only by name", arg: "only-now", want: "now/only-now.org"},
		{name: "same tier exact is ambiguous", arg: "report", ambiguous: []string{"a/report.org", "b/report.org"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &Resolver{Dir: dir}
			path, err := r.Resolve(tc.arg)
			if tc.ambiguous != nil {
				var ambiguity *AmbiguousNoteError
				if !errors.As(err, &ambiguity) || !reflect.DeepEqual(ambiguity.Candidates, tc.ambiguous) {
					t.Fatalf("ambiguity = %#v, error = %v", ambiguity, err)
				}
				return
			}
			if err != nil || r.Relative(path) != tc.want {
				t.Fatalf("path = %q, error = %v, want %q", path, err, tc.want)
			}
		})
	}
}

func TestFilesExcluding(t *testing.T) {
	r := &Resolver{Dir: filepath.Join("..", "..", "testdata", "priority")}
	files, err := r.FilesExcluding(map[string]bool{"now": true})
	if err != nil {
		t.Fatal(err)
	}
	var rel []string
	for _, path := range files {
		rel = append(rel, r.Relative(path))
	}
	want := []string{"a/report.org", "b/report.org", "same.org"}
	if !reflect.DeepEqual(rel, want) {
		t.Fatalf("files = %v, want %v", rel, want)
	}
	// Exclusion must not hide files from note resolution.
	if path, err := r.Resolve("only-now"); err != nil || r.Relative(path) != "now/only-now.org" {
		t.Fatalf("resolution after exclusion: path = %q, error = %v", path, err)
	}
}

func TestUnreadableIDFile(t *testing.T) {
	dir := t.TempDir()
	// A dangling symlink reliably exercises unreadable files even as root.
	if err := os.Symlink(filepath.Join(dir, "absent"), filepath.Join(dir, "broken.org")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	var warnings bytes.Buffer
	r := &Resolver{Dir: dir, Warn: &warnings}
	if _, err := r.Resolve("absent-id"); err == nil || !strings.Contains(warnings.String(), "broken.org") {
		t.Fatalf("error = %v, warnings = %q", err, warnings.String())
	}
}
