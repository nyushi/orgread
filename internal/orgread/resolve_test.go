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
		{name: "ambiguous prefix", arg: "20261018161214", ambiguous: []string{"20261018161214-alpha.org", "nested/20261018161214-beta.org"}},
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
		"name.org":           "* Exact\n",
		"name-extra.org":     "* Prefix\n:ID: name\n:ID: shared\n",
		"id.org":             "* ID\n  :iD: Unique-ID  \n:ID: shared\n",
		"duplicate/name.org": "* Duplicate exact\n",
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
		{"name", "", true},
		{"shared", "", true},
		{"Unique-ID", "id.org", false},
		{"name-e", "name-extra.org", false},
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
				t.Fatalf("path = %q, error = %v", path, err)
			}
		})
	}
	if err := os.Remove(filepath.Join(dir, "duplicate", "name.org")); err != nil {
		t.Fatal(err)
	}
	r := &Resolver{Dir: dir}
	path, err := r.Resolve("name")
	if err != nil || filepath.Base(path) != "name.org" {
		t.Fatalf("exact must win over prefix and ID: %s, %v", path, err)
	}
	// An explicit path wins even when the target directory cannot be read.
	r = &Resolver{Dir: filepath.Join(dir, "missing")}
	if _, err := r.Resolve(filepath.Join(dir, "name.org")); err != nil {
		t.Fatal(err)
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
