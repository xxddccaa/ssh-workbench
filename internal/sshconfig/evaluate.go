package sshconfig

import "strings"

// maximumIdentityFiles matches the OpenSSH client limit.
const maximumIdentityFiles = 100

// setting is one directive value that applies to the alias being evaluated.
// order records when it was obtained so precedence between different
// directives can be compared.
type setting struct {
	key    string
	value  string
	source string
	line   int
	order  int
}

// evaluation keeps only what candidates need: the first value of each
// directive, as OpenSSH uses it, and the IdentityFile values in order. This
// bounds memory regardless of how many directives apply.
type evaluation struct {
	alias string
	// declaration is the first applying Host line that names the alias
	// literally; an alias without one is not usable under that name.
	declaration   setting
	declared      bool
	values        map[string]setting
	identityFiles []setting
	obtained      int
}

// evaluate walks the files the way OpenSSH readconf.c does: blocks apply in
// file order, an Include contributes only when its enclosing block applies,
// and Host lines inside an included file govern only that file.
func evaluate(root *configFile, alias string) *evaluation {
	result := &evaluation{alias: alias, values: map[string]setting{}}
	result.visit(root)
	return result
}

func (evaluation *evaluation) visit(file *configFile) {
	for _, current := range file.blocks {
		if !evaluation.applies(current) {
			continue
		}
		if !evaluation.declared && current.kind == blockHost && namesLiterally(current, evaluation.alias) {
			evaluation.declaration = setting{key: "host", source: file.source, line: current.line}
			evaluation.declared = true
		}
		for _, item := range current.entries {
			if item.include == nil {
				evaluation.record(setting{key: item.key, value: item.value, source: file.source, line: item.line})
				continue
			}
			for _, included := range item.include.files {
				evaluation.visit(included)
			}
		}
	}
}

// record keeps one IdentityFile beyond the limit so the overflow can be
// reported.
func (evaluation *evaluation) record(obtained setting) {
	evaluation.obtained++
	obtained.order = evaluation.obtained
	if obtained.key == "identityfile" && len(evaluation.identityFiles) <= maximumIdentityFiles {
		evaluation.identityFiles = append(evaluation.identityFiles, obtained)
	}
	if _, seen := evaluation.values[obtained.key]; !seen {
		evaluation.values[obtained.key] = obtained
	}
}

func (evaluation *evaluation) applies(current block) bool {
	switch current.kind {
	case blockImplicit, blockMatchAll:
		return true
	case blockHost:
		return current.host.Matches(evaluation.alias)
	case blockMatchHost:
		return current.host.Matches(evaluation.matchHostname())
	default:
		return false
	}
}

// matchHostname is what "Match host" compares against in OpenSSH: the
// HostName obtained so far with %h expanded, or the alias, lowercased.
func (evaluation *evaluation) matchHostname() string {
	hostname := evaluation.alias
	if origin, ok := evaluation.first("hostname"); ok {
		hostname = origin.value
		if expanded, ok := expandTokens(origin.value, map[byte]string{'h': evaluation.alias}); ok {
			hostname = expanded
		}
	}
	return strings.ToLower(hostname)
}

func (evaluation *evaluation) first(key string) (setting, bool) {
	value, ok := evaluation.values[key]
	return value, ok
}

// firstOf returns the earliest of several mutually exclusive directives, such
// as ProxyJump and ProxyCommand where the first one obtained wins.
func (evaluation *evaluation) firstOf(keys ...string) (setting, bool) {
	var earliest setting
	found := false
	for _, key := range keys {
		if value, ok := evaluation.values[key]; ok && (!found || value.order < earliest.order) {
			earliest, found = value, true
		}
	}
	return earliest, found
}

type declaration struct {
	alias  string
	source string
	line   int
}

// declarations lists literal Host aliases in file order, descending into
// included files where the Include appears. Wildcard and negated patterns
// only contribute values.
func declarations(root *configFile) []declaration {
	var found []declaration
	seen := map[string]bool{}
	var walk func(file *configFile)
	walk = func(file *configFile) {
		for _, current := range file.blocks {
			if current.kind == blockHost {
				for _, pattern := range current.host.Patterns {
					alias := pattern.String()
					if isLiteral(alias) && !seen[alias] {
						seen[alias] = true
						found = append(found, declaration{alias: alias, source: file.source, line: current.line})
					}
				}
			}
			for _, item := range current.entries {
				if item.include != nil {
					for _, included := range item.include.files {
						walk(included)
					}
				}
			}
		}
	}
	walk(root)
	return found
}

func isLiteral(pattern string) bool {
	return pattern != "" && !strings.ContainsAny(pattern, "*?!")
}

func namesLiterally(current block, alias string) bool {
	for _, pattern := range current.host.Patterns {
		if pattern.String() == alias {
			return true
		}
	}
	return false
}
