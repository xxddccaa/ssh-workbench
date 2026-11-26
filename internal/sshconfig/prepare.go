package sshconfig

import (
	"fmt"
	"strings"
)

// The library follows Include directives itself and rejects most Match
// criteria, so those lines are rewritten before it sees the text. Every
// rewrite keeps the line count so library positions stay correct.
const (
	includeMarker    = "sshwb-include"
	inertMatchPrefix = "sshwb-unsupported-match-"
)

type blockKind int

const (
	blockImplicit blockKind = iota
	blockHost
	blockMatchAll
	blockMatchHost
	// blockInert replaces a Match block whose criteria are not supported. It
	// never applies.
	blockInert
)

type blockLayout struct {
	kind blockKind
	line int
}

// preparedText records, in file order, what each Host or Match line became so
// that library blocks can be paired with it: library block i+1 is blocks[i],
// block 0 being the implicit leading "Host *".
type preparedText struct {
	text        []byte
	blocks      []blockLayout
	includes    map[int][]string
	diagnostics []Diagnostic
}

func prepare(content []byte) (preparedText, error) {
	text := strings.TrimPrefix(string(content), "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	// OpenSSH treats a stray carriage return as whitespace; the library would
	// treat it as a line break without counting a line.
	text = strings.ReplaceAll(text, "\r", " ")
	lines := strings.Split(text, "\n")
	prepared := preparedText{includes: map[int][]string{}}
	for index, line := range lines {
		number := index + 1
		keyword, value, ok := splitDirective(line)
		if len(line) > maximumLineLength {
			if ok {
				return preparedText{}, fmt.Errorf("%w: line %d is longer than %d bytes", ErrTooLarge, number, maximumLineLength)
			}
			lines[index] = ""
			continue
		}
		if !ok {
			continue
		}
		switch strings.ToLower(keyword) {
		case "include":
			prepared.includes[number] = includePatterns(value)
			lines[index] = "#" + includeMarker
		case "host":
			lines[index] = strings.TrimSpace("Host " + strings.Join(arguments(value), " "))
			prepared.blocks = append(prepared.blocks, blockLayout{kind: blockHost, line: number})
		case "match":
			kind, replacement, diagnostic := prepareMatch(value, len(prepared.blocks))
			lines[index] = replacement
			prepared.blocks = append(prepared.blocks, blockLayout{kind: kind, line: number})
			if diagnostic != nil {
				diagnostic.Line = number
				prepared.diagnostics = append(prepared.diagnostics, *diagnostic)
			}
		}
	}
	prepared.text = []byte(strings.Join(lines, "\n"))
	return prepared, nil
}

// splitDirective mirrors the library lexer: the keyword ends at a space, tab
// or '=', one optional '=' separates it from the value, and '#' starts a
// comment anywhere in the value.
func splitDirective(line string) (keyword string, value string, ok bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" || trimmed[0] == '#' {
		return "", "", false
	}
	end := strings.IndexAny(trimmed, " \t=")
	if end < 0 {
		return trimmed, "", true
	}
	keyword = trimmed[:end]
	value = strings.TrimLeft(trimmed[end:], " \t")
	value = strings.TrimLeft(strings.TrimPrefix(value, "="), " \t")
	if comment := strings.IndexByte(value, '#'); comment >= 0 {
		value = value[:comment]
	}
	return keyword, strings.TrimRight(value, " \t"), true
}

// arguments splits on the whitespace OpenSSH recognises; the library itself
// only splits Host patterns on spaces.
func arguments(value string) []string {
	return strings.FieldsFunc(value, func(character rune) bool {
		return character == ' ' || character == '\t'
	})
}

func includePatterns(value string) []string {
	patterns := arguments(value)
	for index, pattern := range patterns {
		if len(pattern) >= 2 && pattern[0] == '"' && pattern[len(pattern)-1] == '"' {
			patterns[index] = pattern[1 : len(pattern)-1]
		}
	}
	return patterns
}

// prepareMatch lets "Match all" and "Match host <pattern-list>" through and
// turns every other Match line into a Host line that cannot match an alias.
// The comma-separated pattern list becomes space-separated patterns, which
// the library evaluates with the same negation rules. OpenSSH compares
// Match host case-insensitively, so patterns are lowercased here and the
// hostname in the evaluator.
func prepareMatch(value string, sequence int) (blockKind, string, *Diagnostic) {
	criteria := arguments(value)
	if len(criteria) == 1 && strings.EqualFold(criteria[0], "all") {
		return blockMatchAll, "Match all", nil
	}
	if len(criteria) == 2 && strings.EqualFold(criteria[0], "host") {
		patterns := strings.FieldsFunc(strings.ToLower(criteria[1]), func(character rune) bool { return character == ',' })
		if len(patterns) > 0 {
			return blockMatchHost, "Match host " + strings.Join(patterns, " "), nil
		}
	}
	replacement := fmt.Sprintf("Host %s%d", inertMatchPrefix, sequence)
	if mentionsExec(criteria) {
		return blockInert, replacement, &Diagnostic{
			Level:     LevelBlocked,
			Directive: "Match",
			Message:   "Match exec 条件永远不会被执行，该块未导入。",
		}
	}
	return blockInert, replacement, &Diagnostic{
		Level:     LevelWarning,
		Directive: "Match",
		Message:   "条件 Match 块未导入。",
	}
}

func mentionsExec(criteria []string) bool {
	for _, criterion := range criteria {
		if strings.EqualFold(strings.TrimPrefix(criterion, "!"), "exec") {
			return true
		}
	}
	return false
}
