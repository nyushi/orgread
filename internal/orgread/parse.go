// Package orgread reads Org documents without changing their original bytes.
package orgread

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode"
)

const PreambleTitle = "(最初の見出しより前)"

// Line offsets include the newline, when present. Text excludes LF and one CR.
type Line struct {
	StartByte, EndByte int
	Text               []byte
	Section            int // zero-based section position; -1 in an empty document
}

type Section struct {
	Index, Level                  int
	Title, ParentTitle, Path      string
	Parent                        int // zero-based section position, or -1
	StartLine, EndLine, LineCount int
	StartByte, EndByte, Bytes     int
	OwnEndByte                    int // exclusive end before the first child
}

type Document struct {
	Data     []byte
	Lines    []Line
	Sections []Section
}

// Parse retains data. Callers must not mutate it while using the document.
func Parse(data []byte) *Document {
	d := &Document{Data: data}
	for start := 0; start < len(data); {
		end := len(data)
		if n := bytes.IndexByte(data[start:], '\n'); n >= 0 {
			end = start + n + 1
		}
		text := bytes.TrimSuffix(data[start:end], []byte{'\n'})
		text = bytes.TrimSuffix(text, []byte{'\r'})
		d.Lines = append(d.Lines, Line{StartByte: start, EndByte: end, Text: text, Section: -1})
		start = end
	}
	var stack []int
	current := -1
	block := ""
	closeSection := func(index, endLine, endByte int) {
		s := &d.Sections[index]
		s.EndLine, s.EndByte = endLine, endByte
		s.LineCount, s.Bytes = endLine-s.StartLine+1, endByte-s.StartByte
		if s.OwnEndByte < 0 {
			s.OwnEndByte = endByte
		}
	}
	for i := range d.Lines {
		line := &d.Lines[i]
		trimmed := strings.ToLower(strings.TrimSpace(string(line.Text)))
		level, title := 0, ""
		if block != "" {
			if blockType(trimmed, "#+end_") == block {
				block = ""
			}
		} else if kind := blockType(trimmed, "#+begin_"); kind != "" {
			block = kind
		} else {
			level, title = heading(line.Text)
		}
		if level > 0 {
			if len(d.Sections) == 0 && i > 0 {
				d.Sections = append(d.Sections, Section{
					Index: 1, Title: PreambleTitle, Path: PreambleTitle, Parent: -1,
					StartLine: 1, OwnEndByte: -1,
				})
				closeSection(0, i, line.StartByte)
				for j := 0; j < i; j++ {
					d.Lines[j].Section = 0
				}
			}
			for len(stack) > 0 && d.Sections[stack[len(stack)-1]].Level >= level {
				closeSection(stack[len(stack)-1], i, line.StartByte)
				stack = stack[:len(stack)-1]
			}
			parent, parentTitle, path := -1, "", title
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
				p := &d.Sections[parent]
				parentTitle, path = p.Title, p.Path+" > "+title
				if p.OwnEndByte < 0 {
					p.OwnEndByte = line.StartByte
				}
			}
			current = len(d.Sections)
			d.Sections = append(d.Sections, Section{
				Index: current + 1, Level: level, Title: title, ParentTitle: parentTitle,
				Path: path, Parent: parent, StartLine: i + 1, StartByte: line.StartByte, OwnEndByte: -1,
			})
			stack = append(stack, current)
		}
		line.Section = current
	}
	for _, index := range stack {
		closeSection(index, len(d.Lines), len(data))
	}
	if len(d.Sections) == 0 && len(d.Lines) > 0 {
		d.Sections = append(d.Sections, Section{
			Index: 1, Title: PreambleTitle, Path: PreambleTitle, Parent: -1,
			StartLine: 1, OwnEndByte: -1,
		})
		closeSection(0, len(d.Lines), len(data))
		for i := range d.Lines {
			d.Lines[i].Section = 0
		}
	}
	return d
}

func blockType(line, prefix string) string {
	if !strings.HasPrefix(line, prefix) {
		return ""
	}
	fields := strings.Fields(strings.TrimPrefix(line, prefix))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func heading(line []byte) (int, string) {
	i := 0
	for i < len(line) && line[i] == '*' {
		i++
	}
	if i == 0 || i >= len(line) || line[i] != ' ' {
		return 0, ""
	}
	return i, strings.TrimRightFunc(string(line[i+1:]), unicode.IsSpace)
}

// Select returns all matches; numeric selectors always refer to global indices.
func (d *Document) Select(selector string) []Section {
	digits := selector != ""
	for _, c := range selector {
		if c < '0' || c > '9' {
			digits = false
			break
		}
	}
	if digits {
		// Comparing decimal strings avoids overflow for arbitrary user input.
		selector = strings.TrimLeft(selector, "0")
		for _, s := range d.Sections {
			if fmt.Sprint(s.Index) == selector {
				return []Section{s}
			}
		}
		return nil
	}
	if level, title := heading([]byte(selector)); level > 0 {
		selector = title
	}
	var found []Section
	for _, s := range d.Sections {
		if s.Level > 0 && s.Title == selector {
			found = append(found, s)
		}
	}
	return found
}

func (d *Document) Body(s Section, noChildren bool) []byte {
	end := s.EndByte
	if noChildren {
		end = s.OwnEndByte
	}
	return d.Data[s.StartByte:end]
}

func (d *Document) Subtree(s Section) []Section {
	end := s.Index
	if s.Level > 0 {
		for end < len(d.Sections) && d.Sections[end].Level > s.Level {
			end++
		}
	}
	return d.Sections[s.Index-1 : end]
}

func FormatBytes(n int) string {
	kb := float64(n) / 1024
	if kb < 10 {
		return fmt.Sprintf("%.1fKB", kb)
	}
	return fmt.Sprintf("%.0fKB", kb)
}

// WriteTOC uses rune widths (rather than terminal-specific display widths).
func WriteTOC(w io.Writer, sections []Section) error {
	for _, s := range sections {
		title := s.Title
		if s.Level > 0 {
			title = strings.Repeat("*", s.Level) + " " + title
		}
		if s.Parent >= 0 {
			title = fmt.Sprintf("%-28s (親: %s)", title, s.ParentTitle)
		}
		if _, err := fmt.Fprintf(w, "[%d]  %-12s %-7s %6s  %s\n", s.Index,
			fmt.Sprintf("L%d-%d", s.StartLine, s.EndLine), fmt.Sprintf("%d行", s.LineCount),
			FormatBytes(s.Bytes), title); err != nil {
			return err
		}
	}
	return nil
}
