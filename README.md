# orgread

A read-only, standard-library-only Go CLI for reading Org notes by section and
searching them with heading context. It emits plain text, preserves original
section bytes, makes no network requests, and creates no cache or index files.
All files in `testdata/` contain fictional content.

## Install and build

Requires Go 1.25 or newer. From this directory:

```sh
go install .
# Or build locally:
go build -o orgread .
```

`go install .` puts the executable in `GOBIN`, or `$(go env GOPATH)/bin` by
default. Add that directory to your shell's `PATH`.
If Git metadata is unavailable (as in the restricted task workspace), use
`go build -buildvcs=false -o orgread .` or `go install -buildvcs=false .`.
In a sandbox with a read-only default Go cache, set `GOCACHE` to a writable
temporary directory when building or testing.

## Commands

```text
orgread [--dir DIR] toc [--depth N] [--dir DIR] <note>
orgread [--dir DIR] get [--max-bytes N] [--force] [--no-children] [--dir DIR] <note> <selector>
orgread [--dir DIR] grep [-i] [-F] [--max N] [--files] [--dir DIR] <pattern> [note...]
```

Options must precede the positional arguments within each command, following
the standard library `flag` package. Use `--` before arguments starting with `-`.
Both `--dir X toc note` and `toc --dir X note` work. Directory precedence is:
command-level `--dir`, global `--dir`, `ORGREAD_DIR`, then the current directory
(`.`). An explicitly empty directory means `.`.

### toc

Prints a section index, inclusive one-based line range, line count, byte size,
heading, and parent title when present. Sections include their descendant
headings and content. The preamble is a separate section when at least one line
precedes the first heading. A non-empty heading-less file has one preamble;
an empty file prints nothing and has no selectable sections.

`--depth N` displays heading levels at most N; level zero is the preamble.
It filters display only: global section numbers remain stable for `get`.
Depth zero shows only a preamble, and negative values are errors. The default
shows all levels.

Actual output from the included sample:

```text
$ orgread --dir testdata/notes toc guide
[1]  L1-2         2行       0.0KB  (最初の見出しより前)
[2]  L3-15        13行      0.2KB  * 手順書
[3]  L8-13        6行       0.1KB  *** README                   (親: 手順書)
[4]  L14-15       2行       0.0KB  ** 確認                        (親: 手順書)
[5]  L16-17       2行       0.0KB  * 繰り返し
[6]  L18-19       2行       0.0KB  * 繰り返し
```

The line-range and line-count columns have minimum widths of 12 and 7 runes;
the size column has minimum width 6. For headings with a parent, the heading
column has minimum width 28. Long values are never clipped. Padding uses Go's
rune widths, so Japanese terminal display alignment can vary by font.
Sizes use bytes / 1024, with one decimal below 10KB and no decimal at or above
10KB, rounded by `fmt`: for example `0.3KB`, `8.2KB`, `31KB`, `152KB`.
Tiny sections can display `0.0KB`; the parser still stores their exact bytes.

```text
$ orgread --dir testdata/notes toc --depth 1 guide
[1]  L1-2         2行       0.0KB  (最初の見出しより前)
[2]  L3-15        13行      0.2KB  * 手順書
[5]  L16-17       2行       0.0KB  * 繰り返し
[6]  L18-19       2行       0.0KB  * 繰り返し
```

### get

A digits-only selector is the global TOC number (leading zeros are accepted).
Other selectors match the exact, case-sensitive heading title; an optional
leading run of stars and a space is accepted, for example `'*** README'`.
The preamble is selectable only by number. A numeric heading title can be
selected with its starred form.

The body includes its heading and descendants by default. `--no-children`
returns only the heading and lines before its first descendant heading. It
leaves preambles and leaf sections intact.

```text
$ orgread --dir testdata/notes get --no-children guide 2
* 手順書
:PROPERTIES:
:ID: fictional-guide-id
:END:
準備を確認。needle
```

Successful body output is a slice of the original file bytes. CRLF, whitespace,
UTF-8 bytes, and an absent final newline are preserved. No header or extra
newline is added. Line byte counts include the original newline when present;
the final line without a newline counts exactly as stored.

`--max-bytes N` defaults to 51200 bytes: **50KB means 50 × 1024 bytes**.
Zero is allowed; negative limits are errors. If the effective body (after
`--no-children`) exceeds the limit, `get` emits the section and descendant TOC
entries plus the actual size, limit, and `--force` hint; it emits no body and
exits 0. `--force` bypasses this check. The size message includes exact bytes
alongside the same KB formatting used by `toc`.

```text
$ orgread --dir testdata/notes get --max-bytes 100 guide 2
[2]  L3-15        13行      0.2KB  * 手順書
[3]  L8-13        6行       0.1KB  *** README                   (親: 手順書)
[4]  L14-15       2行       0.0KB  ** 確認                        (親: 手順書)
この節は 0.2KB (197 bytes)。本文を出すには --force を付けてください (上限 0.1KB / 100 bytes)
```

A missing selector emits an error on stderr and no stdout, exiting 2.
Duplicate heading titles emit only candidate TOC rows on stdout and an error
on stderr, exiting 2. Select one by its global number to retrieve the body.

### grep

Patterns use Go regular expressions, compiled once per invocation. `-i` ignores
case. `-F` uses a literal substring; with `-i`, both the pattern and line are
Unicode-lowercased before comparing. Each matching line counts once even if
the pattern occurs more than once on that line.

```text
$ orgread --dir testdata/notes grep -i --max 2 needle guide
guide.org > 手順書 : 7: 準備を確認。needle
guide.org > 手順書 > README : 9: Alpha needle
全 4 件のうち 2 件を表示。--max で増やせる
```

The exact match format is
`<filename> > <heading path> : <line number>: <line text>`.
Paths join enclosing heading titles with ` > `; a heading line includes its
own title. Preamble lines use `(最初の見出しより前)`. Match text omits the
newline and trailing CR. There is no scanner line-length limit.

With explicit notes, filenames are basenames and notes are searched in argument
order; repeated arguments are searched repeatedly. All arguments are resolved
before output, so a missing or ambiguous note cannot leave partial results.
With no notes, search walks the target directory recursively for case-sensitive
`*.org` names, skips hidden directories, does not follow directory symlinks,
and sorts paths. Output names are paths relative to that directory.

`--max N` defaults to 200 displayed matches, but all matching lines across all
files are counted. When limited, stdout ends with
`全 <total> 件のうち <shown> 件を表示。--max で増やせる`.
Zero prints only the summary when matches exist. Negative limits are errors.

`--files` prints only nonzero per-file matching-line counts, in search order.
It counts everything and is **not limited by `--max`**; no total line is added.

```text
$ orgread --dir testdata/notes grep --files needle
20261018161214-alpha.org: 1件
guide.org: 3件
nested/20261018161214-beta.org: 1件
nested/plain.org: 1件
```

Unreadable files produce warnings on stderr and search continues. If every
candidate file fails to read, exit status is 2; an empty directory has no hits
and exits 1. An inaccessible root directory is an error; inaccessible descendant
directories are warned about and skipped.

## Note resolution

`toc`, `get`, and explicit `grep` notes share this ordered resolution:

1. A regular file path relative to the current working directory (or absolute),
   then that path with `.org` appended. Explicit paths take precedence over
   `--dir`, and can name files outside the target directory.
2. An exact basename under the target directory, with `.org` optional.
3. A basename prefix under that directory, for example `20261018161214-a`.
4. An exact `:ID:` property value found anywhere in an Org file. Whitespace
   around the marker and value is ignored, and the `:ID:` marker is
   case-insensitive; the value remains case-sensitive.

Steps 2–4 recurse over the same sorted `.org` list, skipping hidden directories.
Filenames and prefixes are case-sensitive. Each stage gathers all matches and
stops on either a unique result or ambiguity; later stages do not break ties.
Ambiguous notes exit 2 and list relative candidate paths, one per line, on
stderr. Missing notes report `見つかりません: <note>` on stderr and exit 2.
Unreadable files encountered while scanning IDs are warned about and skipped.

For the sample, `guide`, `guide.org`, and `fictional-guide-id` resolve to
`guide.org`; `20261018161214` is ambiguous between the alpha and beta notes.

## Parsing and implementation decisions

The root `main.go` contains flag parsing and dispatch through a testable
`run(stdout, stderr, args)` function. `internal/orgread` contains the parser,
note resolver, TOC formatter, and search. No external dependencies are used.

**No Org parser library, including `github.com/niklasfasching/go-org`, is used.**
The byte-identical `get` requirement rules out parsing followed by
re-serialization or normalization, which cannot guarantee the original text.
Block-aware heading detection is simple line scanning. The parser retains the
original `[]byte`, records each line's byte offsets, and computes section ranges
with a heading stack. `get` writes the original slice directly to its output
writer (`os.Stdout` in the CLI), never reconstructing text.

Outside blocks, headings must start at column one with one or more `*` followed
by an ASCII space; `*bold*` and stars followed by a tab are ordinary content.
Trailing title whitespace is removed for matching and display only. Skipped
levels are valid: the nearest previous heading with a strictly smaller level is
the parent, so `*** README` can be a child of `* 手順書`.

Trimmed, case-insensitive `#+begin_TYPE` opens a block, and `#+end_TYPE` with the
same case-insensitive type closes it. Types are the first token after the
prefix; opening arguments are ignored. Block delimiters are ordinary content.
Other begin/end types inside an open block are treated as content, without
nesting. An unmatched opener hides headings through EOF. This supports source,
example, quote, and other named block types.

**Output is never silently truncated.** Oversized `get` requests produce a
size/limit explanation and `--force` hint instead of partial bodies. Limited
`grep` output states the total, shown count, and `--max` hint. TOC depth is an
explicit level filter with stable numbering; candidate listings and TOC titles
are never clipped.

Search reads one file at a time and scans its lines, without goroutines or
persistent indexes. The included 400-file / roughly 8MB benchmark includes file
reads, parsing, counting all matches, and formatting the first 200 results to a
discard writer. A local three-iteration run averaged about 157ms per search,
below the 1-second target; timings depend on hardware and filesystem.

## Exit status and diagnostics

| Status | Meaning |
| --- | --- |
| 0 | Successful `toc`/`get`; `grep` found at least one matching line; help. A size-limited `get` is also 0. |
| 1 | `grep` found no matches. |
| 2 | Bad arguments, invalid regex, missing/ambiguous note or selector, or fatal read/write error. |

Top-level and command `--help` print usage to stderr and exit 0. No arguments
print usage to stderr and exit 2. Argument/flag errors print usage to stderr;
resolution and IO errors print diagnostics there. Section candidates from
`get` are the deliberate stdout exception described above.

## Verification

```sh
gofmt -w main.go main_test.go internal/orgread/*.go
gofmt -l .               # must print nothing
go vet ./...
go test ./...
go test ./internal/orgread -run '^$' -bench BenchmarkGrep400Files8MB -benchtime=3x -benchmem
```

Table-driven internal tests verify block handling, section metadata, preambles,
UTF-8 byte sizes, CRLF, EOF without a newline, empty files, parentage, selectors,
resolution precedence and ambiguity, search modes, output limits, and read
failures. Root CLI tests exercise `run`, byte-for-byte `get` comparisons,
directory precedence, help and errors, global numbering, the default 51200-byte
limit, and output failures. Fixtures live in `testdata/blocks`, `testdata/edges`,
and `testdata/notes`, including a hidden directory excluded from recursive search.

The examples above were captured from a built executable on these fixtures.
