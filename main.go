package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/nyushi/orgread/internal/orgread"
)

const usage = `Usage: orgread [--dir DIR] <command> [options] arguments
  toc  [--depth N] [--dir DIR] <note>
  get  [--max-bytes N] [--force] [--no-children] [--dir DIR] <note> <selector>
  grep [-i] [-F] [--max N] [--files] [--exclude-dir DIR] [--dir DIR] <pattern> [note...]
Target directory: --dir, then ORGREAD_DIR, then .
grep without notes skips directories named by --exclude-dir (repeatable) or
ORGREAD_EXCLUDE_DIRS (comma-separated).
Place command options before positional arguments. Use -- before arguments starting with -.
`

// stringListFlag collects a repeatable string flag such as --exclude-dir.
type stringListFlag []string

func (f *stringListFlag) String() string { return strings.Join(*f, ",") }
func (f *stringListFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func main() {
	os.Exit(run(os.Stdout, os.Stderr, os.Args[1:]))
}

func run(stdout, stderr io.Writer, args []string) int {
	dir := os.Getenv("ORGREAD_DIR")
	if dir == "" {
		dir = "."
	}
	global := flag.NewFlagSet("orgread", flag.ContinueOnError)
	global.SetOutput(stderr)
	global.Usage = func() { fmt.Fprint(stderr, usage) }
	global.StringVar(&dir, "dir", dir, "directory containing Org notes")
	if err := global.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	args = global.Args()
	if len(args) == 0 {
		global.Usage()
		return 2
	}
	command := args[0]
	if command != "toc" && command != "get" && command != "grep" {
		fmt.Fprintf(stderr, "不明なコマンド: %s\n", command)
		global.Usage()
		return 2
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	fs.StringVar(&dir, "dir", dir, "directory containing Org notes")
	var depth, maxBytes, maxMatches int
	var force, noChildren, ignoreCase, fixed, filesOnly bool
	var excludeDirs stringListFlag
	switch command {
	case "toc":
		fs.IntVar(&depth, "depth", math.MaxInt, "maximum heading level")
	case "get":
		fs.IntVar(&maxBytes, "max-bytes", 50*1024, "maximum body size in bytes")
		fs.BoolVar(&force, "force", false, "output body regardless of size")
		fs.BoolVar(&noChildren, "no-children", false, "exclude descendant headings and their bodies")
	case "grep":
		fs.BoolVar(&ignoreCase, "i", false, "ignore case")
		fs.BoolVar(&fixed, "F", false, "match a literal substring")
		fs.IntVar(&maxMatches, "max", 200, "maximum displayed matching lines")
		fs.BoolVar(&filesOnly, "files", false, "show matching line counts per file")
		fs.Var(&excludeDirs, "exclude-dir", "directory name to skip when searching without notes (repeatable)")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	fail := func(err error) int {
		fmt.Fprintln(stderr, err)
		return 2
	}
	badArgs := func() int {
		fmt.Fprintln(stderr, "引数または上限の指定が正しくありません")
		fs.Usage()
		return 2
	}
	args = fs.Args()
	if (command == "toc" && (len(args) != 1 || depth < 0)) ||
		(command == "get" && (len(args) != 2 || maxBytes < 0)) ||
		(command == "grep" && (len(args) < 1 || maxMatches < 0)) {
		return badArgs()
	}
	if dir == "" {
		dir = "."
	}
	resolver := &orgread.Resolver{Dir: dir, Warn: stderr}
	if command == "grep" {
		var files []orgread.SearchFile
		if len(args) == 1 {
			exclude := map[string]bool{}
			for _, name := range strings.Split(os.Getenv("ORGREAD_EXCLUDE_DIRS"), ",") {
				if name = strings.TrimSpace(name); name != "" {
					exclude[name] = true
				}
			}
			for _, name := range excludeDirs {
				if name = strings.TrimSpace(name); name != "" {
					exclude[name] = true
				}
			}
			paths, err := resolver.FilesExcluding(exclude)
			if err != nil {
				return fail(err)
			}
			for _, path := range paths {
				files = append(files, orgread.SearchFile{Path: path, Name: resolver.Relative(path)})
			}
		} else {
			// Resolve every argument before producing output, avoiding partial results
			// on a later ambiguous or missing note.
			for _, note := range args[1:] {
				path, err := resolver.Resolve(note)
				if err != nil {
					return fail(err)
				}
				files = append(files, orgread.SearchFile{Path: path, Name: filepath.Base(path)})
			}
		}
		total, err := orgread.Search(stdout, stderr, files, args[0], orgread.SearchOptions{
			IgnoreCase: ignoreCase, Fixed: fixed, FilesOnly: filesOnly, Max: maxMatches,
		})
		if err != nil {
			return fail(err)
		}
		if total == 0 {
			return 1
		}
		return 0
	}
	path, err := resolver.Resolve(args[0])
	if err != nil {
		return fail(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fail(err)
	}
	doc := orgread.Parse(data)
	if command == "toc" {
		var visible []orgread.Section
		for _, section := range doc.Sections {
			if section.Level <= depth {
				visible = append(visible, section)
			}
		}
		if err := orgread.WriteTOC(stdout, visible); err != nil {
			return fail(err)
		}
		return 0
	}
	sections := doc.Select(args[1])
	if len(sections) == 0 {
		return fail(fmt.Errorf("節が見つかりません: %s", args[1]))
	}
	if len(sections) > 1 {
		if err := orgread.WriteTOC(stdout, sections); err != nil {
			return fail(err)
		}
		return fail(fmt.Errorf("見出しが曖昧です: %s (番号で選択してください)", args[1]))
	}
	section := sections[0]
	body := doc.Body(section, noChildren)
	if len(body) > maxBytes && !force {
		if err := orgread.WriteTOC(stdout, doc.Subtree(section)); err != nil {
			return fail(err)
		}
		_, err := fmt.Fprintf(stdout, "この節は %s (%d bytes)。本文を出すには --force を付けてください (上限 %s / %d bytes)\n",
			orgread.FormatBytes(len(body)), len(body), orgread.FormatBytes(maxBytes), maxBytes)
		if err != nil {
			return fail(err)
		}
		return 0
	}
	// Writing the original slice preserves CRLF, UTF-8, and the absence of a final LF.
	if n, err := stdout.Write(body); err != nil {
		return fail(err)
	} else if n != len(body) {
		return fail(io.ErrShortWrite)
	}
	return 0
}
