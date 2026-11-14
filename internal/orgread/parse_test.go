package orgread

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestBlocks(t *testing.T) {
	for _, tc := range []struct {
		file   string
		titles []string
	}{
		{"src.org", []string{"Real", "Child"}},
		{"example.org", []string{"Real", "Child"}},
		{"uppercase.org", []string{"Real", "Child"}},
		{"unclosed.org", []string{"Real"}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			doc := Parse(fixture(t, "blocks/"+tc.file))
			var titles []string
			for _, s := range doc.Sections {
				titles = append(titles, s.Title)
			}
			if !reflect.DeepEqual(titles, tc.titles) {
				t.Fatalf("headings = %q, want %q", titles, tc.titles)
			}
			if doc.Lines[2].Section != 0 {
				t.Fatal("block line must retain enclosing heading")
			}
		})
	}
}

func TestSectionRanges(t *testing.T) {
	type wantSection struct {
		title                          string
		level, parent                  int
		start, end, startByte, endByte int
		ownEnd                         int
	}
	for _, tc := range []struct {
		file string
		want []wantSection
	}{
		{"notes/guide.org", []wantSection{
			{PreambleTitle, 0, -1, 1, 2, 0, 47, 47},
			{"手順書", 1, -1, 3, 15, 47, 244, 127},
			{"README", 3, 1, 8, 13, 127, 214, 214},
			{"確認", 2, 1, 14, 15, 214, 244, 244},
			{"繰り返し", 1, -1, 16, 17, 244, 278, 278},
			{"繰り返し", 1, -1, 18, 19, 278, 309, 309},
		}},
		{"edges/crlf.org", []wantSection{
			{PreambleTitle, 0, -1, 1, 1, 0, 8, 8},
			{"日本語", 1, -1, 2, 5, 8, 52, 37},
			{"子", 3, 1, 4, 5, 37, 52, 52},
		}},
		{"edges/headingless.org", []wantSection{{PreambleTitle, 0, -1, 1, 2, 0, 20, 20}}},
		{"edges/no-final-newline.org", []wantSection{
			{"First", 1, -1, 1, 4, 0, 26, 13},
			{"Child", 2, 0, 3, 4, 13, 26, 26},
		}},
		{"edges/syntax.org", []wantSection{
			{PreambleTitle, 0, -1, 1, 1, 0, 7, 7},
			{"", 1, -1, 2, 4, 7, 39, 10},
			{"Child", 2, 1, 3, 4, 10, 39, 39},
			{"Next", 1, -1, 5, 5, 39, 46, 46},
		}},
		{"edges/empty.org", nil},
	} {
		t.Run(tc.file, func(t *testing.T) {
			data := fixture(t, tc.file)
			doc := Parse(data)
			if len(doc.Sections) != len(tc.want) {
				t.Fatalf("sections = %d, want %d", len(doc.Sections), len(tc.want))
			}
			for i, want := range tc.want {
				s := doc.Sections[i]
				got := wantSection{s.Title, s.Level, s.Parent, s.StartLine, s.EndLine, s.StartByte, s.EndByte, s.OwnEndByte}
				if got != want || s.Index != i+1 || s.Bytes != want.endByte-want.startByte || s.LineCount != want.end-want.start+1 {
					t.Errorf("section %d = %+v, want %+v", i+1, s, want)
				}
				if !bytes.Equal(doc.Body(s, false), data[want.startByte:want.endByte]) {
					t.Errorf("section %d body changed bytes", i+1)
				}
				if !bytes.Equal(doc.Body(s, true), data[want.startByte:want.ownEnd]) {
					t.Errorf("section %d own body changed bytes", i+1)
				}
				parentTitle := ""
				if want.parent >= 0 {
					parentTitle = doc.Sections[want.parent].Title
				}
				if s.ParentTitle != parentTitle {
					t.Errorf("parent title = %q, want %q", s.ParentTitle, parentTitle)
				}
			}
			for i, line := range doc.Lines {
				if line.Section < 0 || line.Section >= len(doc.Sections) {
					t.Errorf("line %d has invalid section %d", i+1, line.Section)
				}
			}
		})
	}
	data := fixture(t, "edges/crlf.org")
	if len(data) == utf8.RuneCount(data) {
		t.Fatal("Japanese fixture must distinguish bytes from runes")
	}
}

func TestSelect(t *testing.T) {
	doc := Parse(fixture(t, "notes/guide.org"))
	for _, tc := range []struct {
		selector string
		indices  []int
	}{
		{"1", []int{1}}, {"0002", []int{2}}, {"README", []int{3}},
		{"*** README", []int{3}}, {"繰り返し", []int{5, 6}},
		{PreambleTitle, nil}, {"0", nil}, {"999999999999999999999999", nil},
		{"readme", nil}, {"missing", nil},
	} {
		t.Run(tc.selector, func(t *testing.T) {
			var indices []int
			for _, s := range doc.Select(tc.selector) {
				indices = append(indices, s.Index)
			}
			if !reflect.DeepEqual(indices, tc.indices) {
				t.Fatalf("indices = %v, want %v", indices, tc.indices)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want string
	}{{0, "0.0KB"}, {300, "0.3KB"}, {8397, "8.2KB"}, {10240, "10KB"}, {31744, "31KB"}, {155648, "152KB"}} {
		if got := FormatBytes(tc.n); got != tc.want {
			t.Errorf("FormatBytes(%d) = %s, want %s", tc.n, got, tc.want)
		}
	}
}

func TestLineAndBlockEdges(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		lines       int
		titles      []string
	}{
		{"one LF", "\n", 1, []string{PreambleTitle}},
		{"EOF heading", "* End", 1, []string{"End"}},
		{"empty title", "* ", 1, []string{""}},
		{"block before heading", "#+BEGIN_QUOTE\n* hidden\n#+END_QUOTE\n* visible\n", 4, []string{PreambleTitle, "visible"}},
		{"wrong end type", "#+begin_example\n#+end_src\n* hidden\n#+end_example\n* visible", 5, []string{PreambleTitle, "visible"}},
		{"nested begin is content", "#+begin_src\n#+begin_quote\n#+end_quote\n* hidden\n#+end_src\n* visible", 6, []string{PreambleTitle, "visible"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := Parse([]byte(tc.input))
			var titles []string
			for _, s := range doc.Sections {
				titles = append(titles, s.Title)
			}
			if len(doc.Lines) != tc.lines || !reflect.DeepEqual(titles, tc.titles) {
				t.Fatalf("lines = %d, titles = %q", len(doc.Lines), titles)
			}
		})
	}
}

// Benchmark the requested workload, including file IO and output formatting.
func BenchmarkGrep400Files8MB(b *testing.B) {
	dir := b.TempDir()
	data := []byte("* Fictional benchmark\n" + strings.Repeat("A fictional line for searching.\n", 680))
	var files []SearchFile
	for i := 0; i < 400; i++ {
		name := filepath.Join(dir, fmt.Sprintf("note%03d.org", i))
		if err := os.WriteFile(name, data, 0600); err != nil {
			b.Fatal(err)
		}
		files = append(files, SearchFile{Path: name, Name: filepath.Base(name)})
	}
	b.SetBytes(int64(400 * len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Search(io.Discard, io.Discard, files, "fictional", SearchOptions{Max: 200}); err != nil {
			b.Fatal(err)
		}
	}
}
