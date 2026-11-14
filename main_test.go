package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	t.Setenv("ORGREAD_DIR", filepath.Join("testdata", "notes"))
	for _, tc := range []struct {
		name       string
		args       []string
		code       int
		contains   []string
		excludes   []string
		stderr     string
		exact      string
		checkExact bool
	}{
		{name: "no args", code: 2, stderr: "Usage:"},
		{name: "help", args: []string{"--help"}, stderr: "Usage:"},
		{name: "subcommand help", args: []string{"get", "--help"}, stderr: "Usage:"},
		{name: "unknown command", args: []string{"nope"}, code: 2, stderr: "Usage:"},
		{name: "unknown global flag", args: []string{"--wat"}, code: 2, stderr: "Usage:"},
		{name: "unknown flag", args: []string{"get", "--wat"}, code: 2, stderr: "Usage:"},
		{name: "missing args", args: []string{"get", "guide"}, code: 2, stderr: "Usage:"},
		{name: "too many args", args: []string{"toc", "guide", "extra"}, code: 2, stderr: "Usage:"},
		{name: "negative depth", args: []string{"toc", "--depth", "-1", "guide"}, code: 2, stderr: "Usage:"},
		{name: "negative max bytes", args: []string{"get", "--max-bytes", "-1", "guide", "1"}, code: 2, stderr: "Usage:"},
		{name: "negative max", args: []string{"grep", "--max", "-1", "needle"}, code: 2, stderr: "Usage:"},
		{name: "toc global numbering", args: []string{"toc", "--depth", "1", "guide"},
			contains: []string{"[1]", "L1-2", "[2]", "L3-15", "[5]", "[6]"}, excludes: []string{"[3]", "[4]"}},
		{name: "toc depth zero", args: []string{"toc", "--depth", "0", "guide"},
			contains: []string{"[1]", "(最初の見出しより前)"}, excludes: []string{"[2]"}},
		{name: "toc parent suffix", args: []string{"toc", "guide"},
			contains: []string{"*** README", "(親: 手順書)", "13行", "0.2KB"}},
		{name: "empty toc", args: []string{"toc", "testdata/edges/empty.org"}, checkExact: true},
		{name: "missing note", args: []string{"toc", "absent"}, code: 2, stderr: "見つかりません: absent", checkExact: true},
		{name: "prefix prefers direct note", args: []string{"toc", "20261018161214"},
			contains: []string{"[1]", "L1-2", "* Alpha"}},
		{name: "priority file name picks direct", args: []string{"get", "--dir", "testdata/priority", "same", "1"},
			exact: "* 同名ルート\n:PROPERTIES:\n:ID: priority-same-id\n:END:\nルートの本文 needle\n", checkExact: true},
		{name: "priority ID picks direct", args: []string{"get", "--dir", "testdata/priority", "priority-same-id", "1"},
			exact: "* 同名ルート\n:PROPERTIES:\n:ID: priority-same-id\n:END:\nルートの本文 needle\n", checkExact: true},
		{name: "priority prefix picks direct", args: []string{"get", "--dir", "testdata/priority", "sam", "1"},
			exact: "* 同名ルート\n:PROPERTIES:\n:ID: priority-same-id\n:END:\nルートの本文 needle\n", checkExact: true},
		{name: "priority nested only by name", args: []string{"get", "--dir", "testdata/priority", "only-now", "1"},
			exact: "* nowだけ\nnowだけの本文 needle\n", checkExact: true},
		{name: "priority same tier ambiguous", args: []string{"toc", "--dir", "testdata/priority", "report"}, code: 2,
			stderr: "a/report.org\nb/report.org", checkExact: true},
		{name: "grep exclude-dir", args: []string{"grep", "--dir", "testdata/priority", "--exclude-dir", "now", "needle"},
			contains: []string{"same.org > 同名ルート : 5: ルートの本文 needle"}, excludes: []string{"now/"}},
		{name: "grep exclude-dir repeated", args: []string{"grep", "--dir", "testdata/priority", "--exclude-dir", "now", "--exclude-dir", "a", "--exclude-dir", "b", "needle"},
			contains: []string{"same.org > 同名ルート : 5:"}, excludes: []string{"now/", "report.org"}},
		{name: "grep without exclude includes nested", args: []string{"grep", "--dir", "testdata/priority", "needle"},
			contains: []string{"now/same.org > 同名now : 2:", "now/only-now.org > nowだけ : 2:", "same.org > 同名ルート : 5:"}},
		{name: "ambiguous heading", args: []string{"get", "guide", "繰り返し"}, code: 2,
			contains: []string{"[5]", "[6]", "L16-17", "L18-19"}, excludes: []string{"最初の本文", "次の本文"}, stderr: "見出しが曖昧"},
		{name: "missing selector", args: []string{"get", "guide", "absent"}, code: 2, stderr: "節が見つかりません", checkExact: true},
		{name: "preamble title not selectable", args: []string{"get", "guide", "(最初の見出しより前)"}, code: 2, stderr: "節が見つかりません", checkExact: true},
		{name: "oversize", args: []string{"get", "--max-bytes", "1", "guide", "2"},
			contains: []string{"[2]", "[3]", "[4]", "197 bytes", "0.2KB", "1 bytes", "--force"},
			excludes: []string{"準備を確認", "Alpha needle", "[5]"}},
		{name: "own content effective size", args: []string{"get", "--max-bytes", "80", "--no-children", "guide", "2"},
			exact: "* 手順書\n:PROPERTIES:\n:ID: fictional-guide-id\n:END:\n準備を確認。needle\n", checkExact: true},
		{name: "own content oversize", args: []string{"get", "--max-bytes", "79", "--no-children", "guide", "2"},
			contains: []string{"80 bytes", "79 bytes", "[3]", "--force"}, excludes: []string{"準備を確認"}},
		{name: "fixed ignore case", args: []string{"grep", "-F", "-i", "alpha", "guide"},
			exact: "guide.org > 手順書 > README : 9: Alpha needle\n", checkExact: true},
		{name: "get by ID", args: []string{"get", "fictional-guide-id", "4"},
			exact: "** 確認\nNEEDLE を確認。\n", checkExact: true},
		{name: "grep limit across files", args: []string{"grep", "--max", "1", "needle"},
			exact: "20261018161214-alpha.org > Alpha : 2: Fictional alpha needle.\n全 6 件のうち 1 件を表示。--max で増やせる\n", checkExact: true},
		{name: "grep directory relative paths", args: []string{"grep", "needle"},
			contains: []string{"nested/20261018161214-beta.org > Beta : 2:", "nested/plain.org > (最初の見出しより前) : 2:"}, excludes: []string{"secret.org"}},
		{name: "grep explicit basename", args: []string{"grep", "needle", "plain"},
			exact: "plain.org > (最初の見出しより前) : 2: Second line: needle.\n", checkExact: true},
		{name: "grep explicit order", args: []string{"grep", "--files", "needle", "plain", "guide"},
			exact: "plain.org: 1件\nguide.org: 3件\n", checkExact: true},
		{name: "grep files ignores max", args: []string{"grep", "--files", "--max", "0", "needle"},
			exact: "20261018161214-alpha.org: 1件\nguide.org: 3件\nnested/20261018161214-beta.org: 1件\nnested/plain.org: 1件\n", checkExact: true},
		{name: "grep no hits", args: []string{"grep", "absent-pattern"}, code: 1, checkExact: true},
		{name: "grep invalid regex", args: []string{"grep", "["}, code: 2, stderr: "正規表現が無効", checkExact: true},
		{name: "grep no partial output on resolution error", args: []string{"grep", "--dir", "testdata/priority", "needle", "same", "report"}, code: 2,
			stderr: "ノートが曖昧", checkExact: true},
		{name: "global directory", args: []string{"--dir", "testdata/edges", "toc", "headingless"},
			contains: []string{"[1]", "L1-2", "2行", "(最初の見出しより前)"}},
		{name: "command directory overrides global and env", args: []string{"--dir", "absent", "get", "--dir", "testdata/edges", "headingless", "1"},
			exact: "前書き\r\n終わり", checkExact: true},
		{name: "double dash", args: []string{"grep", "-F", "--", "-not-a-flag", "guide"}, code: 1, checkExact: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run(&out, &errOut, tc.args); code != tc.code {
				t.Fatalf("exit = %d, want %d; stdout=%q stderr=%q", code, tc.code, out.String(), errOut.String())
			}
			if tc.checkExact && out.String() != tc.exact {
				t.Errorf("stdout = %q, want %q", out.String(), tc.exact)
			}
			for _, s := range tc.contains {
				if !strings.Contains(out.String(), s) {
					t.Errorf("stdout missing %q: %q", s, out.String())
				}
			}
			for _, s := range tc.excludes {
				if strings.Contains(out.String(), s) {
					t.Errorf("stdout unexpectedly contains %q: %q", s, out.String())
				}
			}
			if tc.stderr == "" && errOut.Len() != 0 {
				t.Errorf("unexpected stderr: %q", errOut.String())
			} else if tc.stderr != "" && !strings.Contains(errOut.String(), tc.stderr) {
				t.Errorf("stderr = %q, want %q", errOut.String(), tc.stderr)
			}
		})
	}
}

func TestGetOriginalBytes(t *testing.T) {
	for _, tc := range []struct {
		name, file, selector string
		flags                []string
		start, end           int
	}{
		{"Japanese preamble", "notes/guide.org", "1", nil, 0, 47},
		{"parent includes descendants", "notes/guide.org", "2", nil, 47, 244},
		{"title with stars", "notes/guide.org", "*** README", nil, 127, 214},
		{"force", "notes/guide.org", "2", []string{"--force", "--max-bytes", "0"}, 47, 244},
		{"limit equality", "notes/guide.org", "2", []string{"--max-bytes", "197"}, 47, 244},
		{"no children", "notes/guide.org", "2", []string{"--no-children"}, 47, 127},
		{"no heading", "edges/headingless.org", "1", nil, 0, 20},
		{"CRLF parent", "edges/crlf.org", "日本語", nil, 8, 52},
		{"CRLF own content", "edges/crlf.org", "2", []string{"--no-children"}, 8, 37},
		{"no final newline", "edges/no-final-newline.org", "2", nil, 13, 26},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("testdata", tc.file)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			args := append([]string{"get"}, tc.flags...)
			args = append(args, path, tc.selector)
			var out, errOut bytes.Buffer
			if code := run(&out, &errOut, args); code != 0 || !bytes.Equal(out.Bytes(), data[tc.start:tc.end]) {
				t.Fatalf("exit=%d, stdout=%q, stderr=%q; want bytes %d:%d", code, out.String(), errOut.String(), tc.start, tc.end)
			}
		})
	}
}

func TestDefaultSizeLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.org")
	data := []byte("* Large\n" + strings.Repeat("Fictional body.\n", 4000) + "** Child\nlast\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run(&out, &errOut, []string{"get", path, "1"}); code != 0 {
		t.Fatalf("exit=%d, stderr=%q", code, errOut.String())
	}
	for _, s := range []string{"[1]", "[2]", "--force", "上限 50KB / 51200 bytes"} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("missing %q in %q", s, out.String())
		}
	}
	if strings.Contains(out.String(), "Fictional body") {
		t.Fatal("size-limited get must not emit body")
	}
	out.Reset()
	if code := run(&out, &errOut, []string{"get", "--force", path, "1"}); code != 0 || !bytes.Equal(out.Bytes(), data) {
		t.Fatalf("forced output differs: exit=%d", code)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestOutputErrors(t *testing.T) {
	for _, args := range [][]string{
		{"toc", "testdata/notes/guide.org"},
		{"get", "testdata/notes/guide.org", "2"},
		{"get", "testdata/notes/guide.org", "繰り返し"},
		{"get", "--max-bytes", "1", "testdata/notes/guide.org", "2"},
		{"grep", "needle", "testdata/notes/guide.org"},
		{"grep", "--files", "needle", "testdata/notes/guide.org"},
		{"grep", "--max", "0", "needle", "testdata/notes/guide.org"},
	} {
		var stderr bytes.Buffer
		if code := run(failingWriter{}, &stderr, args); code != 2 || !strings.Contains(stderr.String(), "output unavailable") {
			t.Errorf("args=%v exit=%d stderr=%q", args, code, stderr.String())
		}
	}
}

func TestDefaultCurrentDirectory(t *testing.T) {
	t.Setenv("ORGREAD_DIR", "")
	var out, stderr bytes.Buffer
	if code := run(&out, &stderr, []string{"grep", "--files", "needle"}); code != 0 || !strings.Contains(out.String(), "testdata/notes/guide.org: 3件") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
}

func TestGrepExcludeDirsEnv(t *testing.T) {
	t.Setenv("ORGREAD_EXCLUDE_DIRS", "now")
	var out, stderr bytes.Buffer
	if code := run(&out, &stderr, []string{"grep", "--dir", "testdata/priority", "needle"}); code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(out.String(), "same.org > 同名ルート : 5:") || strings.Contains(out.String(), "now/") {
		t.Fatalf("ORGREAD_EXCLUDE_DIRS not applied: %q", out.String())
	}
	// The environment variable and repeated flags combine.
	t.Setenv("ORGREAD_EXCLUDE_DIRS", "a, b")
	out.Reset()
	if code := run(&out, &stderr, []string{"grep", "--dir", "testdata/priority", "--exclude-dir", "now", "needle"}); code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if strings.Contains(out.String(), "now/") || strings.Contains(out.String(), "report.org") {
		t.Fatalf("combined exclusions not applied: %q", out.String())
	}
}

func TestEmptySearchDirectory(t *testing.T) {
	var out, stderr bytes.Buffer
	if code := run(&out, &stderr, []string{"grep", "--dir", t.TempDir(), "needle"}); code != 1 || out.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
	if code := run(io.Discard, &stderr, []string{"grep", "--dir", filepath.Join(t.TempDir(), "missing"), "needle"}); code != 2 {
		t.Fatalf("unreadable root exit=%d", code)
	}
}
