package orgread

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

type SearchFile struct {
	Path, Name string
}

type SearchOptions struct {
	IgnoreCase, Fixed, FilesOnly bool
	Max                          int
}

// Search counts matching lines, not individual occurrences. It reads every file
// even when the display limit is reached. Unreadable files are warned and skipped.
func Search(out, warnings io.Writer, files []SearchFile, pattern string, opts SearchOptions) (int, error) {
	if opts.Max < 0 {
		return 0, fmt.Errorf("--max は 0 以上で指定してください")
	}
	var match func([]byte) bool
	if opts.Fixed {
		if opts.IgnoreCase {
			pattern = strings.ToLower(pattern)
			match = func(line []byte) bool { return strings.Contains(strings.ToLower(string(line)), pattern) }
		} else {
			needle := []byte(pattern)
			match = func(line []byte) bool { return bytes.Contains(line, needle) }
		}
	} else {
		if opts.IgnoreCase {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return 0, fmt.Errorf("正規表現が無効です: %w", err)
		}
		match = re.Match
	}
	total, shown, read := 0, 0, 0
	for _, file := range files {
		data, err := os.ReadFile(file.Path)
		if err != nil {
			fmt.Fprintf(warnings, "警告: %s: %v\n", file.Name, err)
			continue
		}
		read++
		doc := Parse(data)
		count := 0
		for i, line := range doc.Lines {
			if !match(line.Text) {
				continue
			}
			total++
			count++
			if !opts.FilesOnly && shown < opts.Max {
				path := doc.Sections[line.Section].Path
				if _, err := fmt.Fprintf(out, "%s > %s : %d: %s\n", file.Name, path, i+1, line.Text); err != nil {
					return total, err
				}
				shown++
			}
		}
		if opts.FilesOnly && count > 0 {
			if _, err := fmt.Fprintf(out, "%s: %d件\n", file.Name, count); err != nil {
				return total, err
			}
		}
	}
	if len(files) > 0 && read == 0 {
		return 0, fmt.Errorf("すべてのファイルを読み込めませんでした")
	}
	if !opts.FilesOnly && total > shown {
		if _, err := fmt.Fprintf(out, "全 %d 件のうち %d 件を表示。--max で増やせる\n", total, shown); err != nil {
			return total, err
		}
	}
	return total, nil
}
