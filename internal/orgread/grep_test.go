package orgread

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearch(t *testing.T) {
	file := SearchFile{Path: filepath.Join("..", "..", "testdata", "notes", "guide.org"), Name: "guide.org"}
	for _, tc := range []struct {
		name, pattern string
		opts          SearchOptions
		count         int
		contains      []string
		excludes      []string
		invalid       bool
	}{
		{name: "regexp paths", pattern: "needle", opts: SearchOptions{Max: 200}, count: 3,
			contains: []string{"guide.org > 手順書 : 7: 準備を確認。needle\n", "guide.org > 手順書 > README : 11: * 偽の見出し needle\n"}},
		{name: "heading own path", pattern: "README", opts: SearchOptions{Max: 200}, count: 1,
			contains: []string{"guide.org > 手順書 > README : 8: *** README\n"}},
		{name: "preamble path", pattern: "前書き", opts: SearchOptions{Max: 200}, count: 1,
			contains: []string{"guide.org > " + PreambleTitle + " : 2: 前書きです。\n"}},
		{name: "limit", pattern: "needle", opts: SearchOptions{IgnoreCase: true, Max: 2}, count: 4,
			contains: []string{"全 4 件のうち 2 件を表示。--max で増やせる\n"}, excludes: []string{"11: * 偽", "15: NEEDLE"}},
		{name: "zero limit", pattern: "needle", opts: SearchOptions{Max: 0}, count: 3,
			contains: []string{"全 3 件のうち 0 件を表示。--max で増やせる\n"}, excludes: []string{"guide.org >"}},
		{name: "fixed case sensitive", pattern: "ALPHA", opts: SearchOptions{Fixed: true, Max: 200}, count: 0},
		{name: "fixed ignore case", pattern: "ALPHA", opts: SearchOptions{Fixed: true, IgnoreCase: true, Max: 200}, count: 1,
			contains: []string{"9: Alpha needle\n"}},
		{name: "fixed punctuation", pattern: "a.b", opts: SearchOptions{Fixed: true, Max: 200}, count: 1},
		{name: "fixed invalid regexp", pattern: "[", opts: SearchOptions{Fixed: true, Max: 200}, count: 0},
		{name: "invalid regexp", pattern: "[", opts: SearchOptions{Max: 200}, invalid: true},
		{name: "negative max", pattern: "needle", opts: SearchOptions{Max: -1}, invalid: true},
		{name: "files ignores limit", pattern: "needle", opts: SearchOptions{FilesOnly: true, Max: 0}, count: 3,
			contains: []string{"guide.org: 3件\n"}, excludes: []string{"全 ", " > "}},
		{name: "no matches", pattern: "nonexistent", opts: SearchOptions{Max: 200}, count: 0},
		{name: "one count per line", pattern: "e", opts: SearchOptions{Max: 200}, count: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, warnings bytes.Buffer
			total, err := Search(&out, &warnings, []SearchFile{file}, tc.pattern, tc.opts)
			if (err != nil) != tc.invalid || total != tc.count {
				t.Fatalf("total = %d, error = %v; want %d, invalid=%v", total, err, tc.count, tc.invalid)
			}
			for _, s := range tc.contains {
				if !strings.Contains(out.String(), s) {
					t.Errorf("output missing %q: %q", s, out.String())
				}
			}
			for _, s := range tc.excludes {
				if strings.Contains(out.String(), s) {
					t.Errorf("output contains unwanted %q: %q", s, out.String())
				}
			}
			if warnings.Len() != 0 {
				t.Errorf("unexpected warnings: %s", &warnings)
			}
		})
	}
}

func TestSearchReadFailures(t *testing.T) {
	file := SearchFile{Path: filepath.Join(t.TempDir(), "missing.org"), Name: "missing.org"}
	good := SearchFile{Path: filepath.Join("..", "..", "testdata", "notes", "guide.org"), Name: "guide.org"}
	for _, tc := range []struct {
		name  string
		files []SearchFile
		fatal bool
	}{{"all fail", []SearchFile{file}, true}, {"some fail", []SearchFile{file, good}, false}, {"no files", nil, false}} {
		t.Run(tc.name, func(t *testing.T) {
			var warnings bytes.Buffer
			_, err := Search(io.Discard, &warnings, tc.files, "needle", SearchOptions{Max: 200})
			if (err != nil) != tc.fatal {
				t.Fatalf("error = %v", err)
			}
			if len(tc.files) > 0 && !strings.Contains(warnings.String(), "警告: missing.org") {
				t.Fatalf("missing warning: %s", &warnings)
			}
		})
	}
}

func TestSearchLongLineCRLF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "long.org")
	line := strings.Repeat("x", 100000) + "NEEDLE"
	if err := os.WriteFile(path, []byte("* Long\r\n"+line+"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	total, err := Search(&out, io.Discard, []SearchFile{{Path: path, Name: "long.org"}}, "needle", SearchOptions{IgnoreCase: true, Max: 1})
	if err != nil || total != 1 || out.String() != "long.org > Long : 2: "+line+"\n" {
		t.Fatalf("long-line search: total=%d, error=%v, output bytes=%d", total, err, out.Len())
	}
}
