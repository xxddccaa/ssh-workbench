package sshconfig

import (
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// directiveNames lists every directive this package interprets, keyed by its
// lowercase form, with the spelling used in diagnostics.
var directiveNames = map[string]string{
	"host":                  "Host",
	"hostname":              "HostName",
	"port":                  "Port",
	"user":                  "User",
	"identityfile":          "IdentityFile",
	"identitiesonly":        "IdentitiesOnly",
	"identityagent":         "IdentityAgent",
	"proxyjump":             "ProxyJump",
	"proxycommand":          "ProxyCommand",
	"localcommand":          "LocalCommand",
	"permitlocalcommand":    "PermitLocalCommand",
	"remotecommand":         "RemoteCommand",
	"knownhostscommand":     "KnownHostsCommand",
	"certificatefile":       "CertificateFile",
	"pkcs11provider":        "PKCS11Provider",
	"securitykeyprovider":   "SecurityKeyProvider",
	"gssapiauthentication":  "GSSAPIAuthentication",
	"hostkeyalias":          "HostKeyAlias",
	"canonicalizehostname":  "CanonicalizeHostname",
	"forwardagent":          "ForwardAgent",
	"localforward":          "LocalForward",
	"remoteforward":         "RemoteForward",
	"dynamicforward":        "DynamicForward",
	"stricthostkeychecking": "StrictHostKeyChecking",
	"userknownhostsfile":    "UserKnownHostsFile",
}

// advisory describes a directive that is recognised but not imported. Only
// the first value applies, as in OpenSSH; for repeatable directives the
// first occurrence is enough to report.
type advisory struct {
	key     string
	level   Level
	message string
	applies func(value string) bool
}

var advisories = []advisory{
	{"localcommand", LevelBlocked, "LocalCommand 不会被执行。", isNotNone},
	{"permitlocalcommand", LevelBlocked, "应用不执行本机命令，PermitLocalCommand 不生效。", isYes},
	{"remotecommand", LevelBlocked, "RemoteCommand 不会被执行，连接后打开普通 shell。", isNotNone},
	{"knownhostscommand", LevelBlocked, "KnownHostsCommand 不会被执行。", isNotNone},
	{"certificatefile", LevelWarning, "证书文件未导入，不支持证书认证。", isNotNone},
	{"pkcs11provider", LevelWarning, "PKCS#11 提供程序不会被加载。", isNotNone},
	{"securitykeyprovider", LevelWarning, "SecurityKeyProvider 不会被加载。", func(value string) bool { return !strings.EqualFold(value, "internal") }},
	{"gssapiauthentication", LevelWarning, "不支持 GSSAPI 认证。", isYes},
	{"hostkeyalias", LevelWarning, "HostKeyAlias 未导入，主机指纹按实际连接地址确认。", always},
	{"canonicalizehostname", LevelWarning, "不支持主机名规范化，HostName 按原样导入。", func(value string) bool { return isYes(value) || strings.EqualFold(value, "always") }},
	{"forwardagent", LevelWarning, "不支持 agent 转发。", func(value string) bool { return !isNo(value) }},
	{"localforward", LevelWarning, "端口转发未导入。", always},
	{"remoteforward", LevelWarning, "远程端口转发未导入。", always},
	{"dynamicforward", LevelWarning, "动态端口转发未导入。", always},
	{"stricthostkeychecking", LevelInfo, "应用始终使用自己的主机指纹信任存储，未知主机须明确确认。", func(value string) bool { return !isYes(value) && !strings.EqualFold(value, "ask") }},
	{"userknownhostsfile", LevelInfo, "应用始终使用自己的主机指纹信任存储，不使用此文件。", always},
}

func always(string) bool { return true }

func isYes(value string) bool {
	return strings.EqualFold(value, "yes") || strings.EqualFold(value, "true")
}

func isNo(value string) bool {
	return strings.EqualFold(value, "no") || strings.EqualFold(value, "false")
}

func isNotNone(value string) bool { return !strings.EqualFold(value, "none") }

func (loader *loader) result(root *configFile) Result {
	result := emptyResult()
	declared := declarations(root)
	if len(declared) > maximumCandidates {
		loader.report(Diagnostic{Level: LevelWarning, Directive: "Host", Message: "主机数量超过 1000 个，只列出前 1000 个。"})
		declared = declared[:maximumCandidates]
	}
	builder := &candidateBuilder{root: root, home: loader.home, evaluations: map[string]*evaluation{}, aliases: map[string]bool{}}
	var reachable []*evaluation
	for _, item := range declared {
		evaluation := builder.evaluation(item.alias)
		if !evaluation.declared {
			loader.report(Diagnostic{Level: LevelInfo, Directive: "Host", Message: "该 Host 块对此别名不生效，未作为主机导入。", Source: item.source, Line: item.line})
			continue
		}
		builder.aliases[item.alias] = true
		reachable = append(reachable, evaluation)
	}
	for _, evaluation := range reachable {
		result.Candidates = append(result.Candidates, builder.build(evaluation))
	}
	result.Diagnostics = append(result.Diagnostics, loader.diagnostics...)
	return result
}

type candidateBuilder struct {
	root        *configFile
	home        string
	evaluations map[string]*evaluation
	aliases     map[string]bool
}

func (builder *candidateBuilder) evaluation(name string) *evaluation {
	if cached, ok := builder.evaluations[name]; ok {
		return cached
	}
	evaluation := evaluate(builder.root, name)
	builder.evaluations[name] = evaluation
	return evaluation
}

type draft struct {
	candidate Candidate
}

func (draft *draft) note(level Level, origin setting, message string) {
	draft.candidate.Diagnostics = append(draft.candidate.Diagnostics, Diagnostic{
		Level:     level,
		Directive: directiveNames[origin.key],
		Message:   message,
		Source:    origin.source,
		Line:      origin.line,
	})
}

func (draft *draft) reject(level Level, origin setting, message string) {
	draft.note(level, origin, message)
	draft.candidate.Importable = false
}

func newDraft(alias string) *draft {
	return &draft{candidate: Candidate{Alias: alias, Importable: true, Diagnostics: []Diagnostic{}}}
}

func (builder *candidateBuilder) build(evaluation *evaluation) Candidate {
	draft := newDraft(evaluation.alias)
	if !validText(evaluation.alias, 253) {
		draft.reject(LevelWarning, evaluation.declaration, "主机别名包含无效字符。")
	}
	candidate := &draft.candidate
	candidate.Hostname, candidate.Port, candidate.User = resolveTarget(evaluation, draft)
	candidate.IdentityFiles = builder.identityFiles(evaluation, draft)
	candidate.IdentitiesOnly = identitiesOnly(evaluation, draft)
	candidate.UseAgent = useAgent(evaluation, draft, candidate.IdentitiesOnly, len(candidate.IdentityFiles) > 0)
	candidate.Jump, candidate.ProxyCommand = builder.routing(evaluation, draft)
	for _, rule := range advisories {
		if origin, ok := evaluation.first(rule.key); ok && rule.applies(origin.value) {
			draft.note(rule.level, origin, rule.message)
		}
	}
	return draft.candidate
}

func resolveTarget(evaluation *evaluation, draft *draft) (string, int, string) {
	return resolveHostname(evaluation, draft), resolvePort(evaluation, draft), resolveUser(evaluation, draft)
}

// resolveHostname supports only the %h token in HostName; any other token
// depends on runtime state the importer cannot reproduce.
func resolveHostname(evaluation *evaluation, draft *draft) string {
	origin, ok := evaluation.first("hostname")
	if !ok {
		if !validHostname(evaluation.alias) {
			draft.reject(LevelWarning, evaluation.declaration, "主机别名不是有效的主机地址，请设置 HostName。")
		}
		return evaluation.alias
	}
	expanded, ok := expandTokens(origin.value, map[byte]string{'h': evaluation.alias})
	if !ok {
		draft.reject(LevelWarning, origin, "HostName 只支持 %h 令牌。")
		return origin.value
	}
	if !validHostname(expanded) {
		draft.reject(LevelWarning, origin, "HostName 不是有效的主机地址。")
	}
	return expanded
}

func resolvePort(evaluation *evaluation, draft *draft) int {
	origin, ok := evaluation.first("port")
	if !ok {
		return 22
	}
	port, ok := parsePort(origin.value)
	if !ok {
		draft.reject(LevelWarning, origin, "Port 必须是 1–65535 之间的数字。")
		return 0
	}
	return port
}

// resolveUser leaves the user empty when it depends on % tokens, so the
// caller asks for it instead of importing a literal token.
func resolveUser(evaluation *evaluation, draft *draft) string {
	origin, ok := evaluation.first("user")
	if !ok {
		return ""
	}
	expanded, ok := expandTokens(origin.value, nil)
	if !ok {
		draft.note(LevelWarning, origin, "User 中的 % 令牌不受支持，未导入用户名。")
		return ""
	}
	if !validText(expanded, 128) {
		draft.reject(LevelWarning, origin, "User 不是有效的用户名。")
		return ""
	}
	return expanded
}

// identityFiles collects every IdentityFile in order without reading them.
// In local mode ~ and %d expand to the home directory; uploaded text keeps
// the ~/ form because no local home applies to it.
func (builder *candidateBuilder) identityFiles(evaluation *evaluation, draft *draft) []string {
	var files []string
	seen := map[string]bool{}
	settings := evaluation.identityFiles
	if len(settings) > maximumIdentityFiles {
		draft.note(LevelWarning, settings[maximumIdentityFiles], "IdentityFile 超过 100 个，其余已忽略。")
		settings = settings[:maximumIdentityFiles]
	}
	for _, origin := range settings {
		if strings.EqualFold(origin.value, "none") {
			continue
		}
		path, ok := builder.expandIdentity(origin.value)
		if !ok {
			draft.note(LevelWarning, origin, "IdentityFile 使用了不支持的令牌或相对路径，已忽略。")
			continue
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		files = append(files, path)
	}
	return files
}

func (builder *candidateBuilder) expandIdentity(value string) (string, bool) {
	home := builder.home
	if home == "" {
		home = "~"
	}
	expanded, ok := expandTokens(value, map[byte]string{'d': home})
	if !ok {
		return "", false
	}
	switch {
	case expanded == "~" || strings.HasPrefix(expanded, "~/"):
		if builder.home == "" {
			return expanded, true
		}
		return filepath.Join(builder.home, expanded[1:]), true
	case filepath.IsAbs(expanded):
		return filepath.Clean(expanded), true
	default:
		return "", false
	}
}

func (builder *candidateBuilder) insideSSHDirectory(path string) bool {
	if builder.home == "" {
		return strings.HasPrefix(path, "~/.ssh/") && !strings.Contains(path, "/../")
	}
	return within(filepath.Join(builder.home, ".ssh"), path) && path != filepath.Join(builder.home, ".ssh")
}

func identitiesOnly(evaluation *evaluation, draft *draft) bool {
	origin, ok := evaluation.first("identitiesonly")
	if !ok {
		return false
	}
	if !isYes(origin.value) && !isNo(origin.value) {
		draft.note(LevelWarning, origin, "IdentitiesOnly 的值无效，按 no 处理。")
	}
	return isYes(origin.value)
}

// useAgent is true unless the configuration rules the agent out. Agent use
// is off for "IdentityAgent none", and when IdentitiesOnly is set with
// explicit identity files: OpenSSH would then only offer agent keys matching
// those files, which the agent auth mode cannot filter, so the explicit key
// is preferred. A custom agent socket is reported and the system agent used.
func useAgent(evaluation *evaluation, draft *draft, identitiesOnly bool, hasIdentityFiles bool) bool {
	if origin, ok := evaluation.first("identityagent"); ok {
		switch {
		case strings.EqualFold(origin.value, "none"):
			return false
		case origin.value != "SSH_AUTH_SOCK" && origin.value != "$SSH_AUTH_SOCK":
			draft.note(LevelWarning, origin, "IdentityAgent 指定的套接字未导入，将使用系统 agent。")
		}
	}
	return !(identitiesOnly && hasIdentityFiles)
}

// routing follows OpenSSH precedence: whichever of ProxyJump and ProxyCommand
// is obtained first wins, and "none" on either means a direct connection.
func (builder *candidateBuilder) routing(evaluation *evaluation, draft *draft) (*Jump, string) {
	origin, ok := evaluation.firstOf("proxyjump", "proxycommand")
	if !ok || strings.EqualFold(origin.value, "none") {
		return nil, ""
	}
	if origin.key == "proxycommand" {
		return nil, origin.value
	}
	if strings.Contains(origin.value, ",") {
		draft.reject(LevelWarning, origin, "只支持单级跳板，多级 ProxyJump 未导入。")
		return nil, ""
	}
	hop, ok := parseHop(origin.value)
	if !ok {
		draft.reject(LevelWarning, origin, "ProxyJump 格式无法识别。")
		return nil, ""
	}
	hopEvaluation := builder.evaluation(hop.host)
	if routed(hopEvaluation) {
		draft.reject(LevelWarning, origin, "只支持单级跳板：跳板机本身不能再经过其他跳板。")
		return nil, ""
	}
	hopDraft := newDraft(hop.host)
	hostname, port, user := resolveTarget(hopEvaluation, hopDraft)
	if !hopDraft.candidate.Importable {
		draft.reject(LevelWarning, origin, "跳板机的地址、端口或用户名无效。")
		return nil, ""
	}
	jump := &Jump{Hostname: hostname, Port: port, User: user}
	if builder.aliases[hop.host] {
		jump.Alias = hop.host
	}
	if hop.user != "" {
		jump.User = hop.user
	}
	if hop.port != 0 {
		jump.Port = hop.port
	}
	return jump, ""
}

func routed(evaluation *evaluation) bool {
	origin, ok := evaluation.firstOf("proxyjump", "proxycommand")
	return ok && !strings.EqualFold(origin.value, "none")
}

type hop struct {
	user string
	host string
	port int
}

// parseHop accepts [user@]host[:port], a bracketed IPv6 address with an
// optional port, and the same forms as an ssh:// URI. Tokens are refused,
// and so is a ':' in the user part, which in URI form would carry a password.
func parseHop(value string) (hop, bool) {
	if rest, isURI := strings.CutPrefix(value, "ssh://"); isURI {
		value = strings.TrimSuffix(rest, "/")
	}
	if strings.Contains(value, "%") {
		return hop{}, false
	}
	var result hop
	if at := strings.LastIndexByte(value, '@'); at >= 0 {
		result.user = value[:at]
		value = value[at+1:]
		if !validText(result.user, 128) || strings.Contains(result.user, ":") {
			return hop{}, false
		}
	}
	host, portText, ok := splitHostPort(value)
	if !ok {
		return hop{}, false
	}
	result.host = host
	if portText != "" {
		if result.port, ok = parsePort(portText); !ok {
			return hop{}, false
		}
	}
	return result, result.host != ""
}

func splitHostPort(value string) (string, string, bool) {
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return value[1 : len(value)-1], "", true
	}
	if !strings.Contains(value, ":") {
		return value, "", true
	}
	host, port, err := net.SplitHostPort(value)
	return host, port, err == nil && port != ""
}

func parsePort(value string) (int, bool) {
	port, err := strconv.ParseUint(value, 10, 16)
	return int(port), err == nil && port > 0
}

// expandTokens replaces %% and the given tokens. It reports false when the
// value uses any other token.
func expandTokens(value string, tokens map[byte]string) (string, bool) {
	if !strings.Contains(value, "%") {
		return value, true
	}
	var expanded strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '%' {
			expanded.WriteByte(value[index])
			continue
		}
		index++
		if index == len(value) {
			return "", false
		}
		if value[index] == '%' {
			expanded.WriteByte('%')
			continue
		}
		replacement, known := tokens[value[index]]
		if !known {
			return "", false
		}
		expanded.WriteString(replacement)
	}
	return expanded.String(), true
}

func validHostname(hostname string) bool {
	if !validText(hostname, 253) || strings.ContainsAny(hostname, "/\\@") {
		return false
	}
	for _, character := range hostname {
		if unicode.IsSpace(character) {
			return false
		}
	}
	return !strings.Contains(hostname, ":") || net.ParseIP(hostname) != nil
}

func validText(value string, maximum int) bool {
	if value == "" || len(value) > maximum*utf8.UTFMax || !utf8.ValidString(value) || utf8.RuneCountInString(value) > maximum {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
