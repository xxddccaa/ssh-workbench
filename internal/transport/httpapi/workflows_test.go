package httpapi_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"sshworkbench/internal/connections"
	"sshworkbench/internal/credentials"
	"sshworkbench/internal/hosts"
	"sshworkbench/internal/tunnels"
	"sshworkbench/tests/fixtures/sshserver"
)

func decodeInto[Value any](test *testing.T, body []byte) Value {
	test.Helper()
	var value Value
	if err := json.Unmarshal(body, &value); err != nil {
		test.Fatalf("decode %s: %v", body, err)
	}
	return value
}

func startFixture(test *testing.T) *sshserver.Server {
	test.Helper()
	server, err := sshserver.Start("")
	if err != nil {
		test.Fatal(err)
	}
	test.Cleanup(server.Close)
	return server
}

func (fixture *harness) createHost(request map[string]any) hosts.Profile {
	fixture.test.Helper()
	body, _ := fixture.call("POST", "/api/v1/hosts", request, 201)
	return decodeInto[hosts.Profile](fixture.test, body)
}

func freePort(test *testing.T) int {
	test.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		test.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

func TestKeyboardInteractiveThroughJumpHostExitStatusAndTunnel(test *testing.T) {
	fixture := newHarness(test)
	jump := startFixture(test)
	target := startFixture(test)
	echo, err := sshserver.StartEcho("")
	if err != nil {
		test.Fatal(err)
	}
	defer echo.Close()
	fixture.authorize()
	jumpHost := fixture.createHost(hostRequest("跳板", jump.Address(), sshserver.Username, "password", nil))
	targetHost := fixture.createHost(hostRequest("MFA 目标", target.Address(), sshserver.MFAUsername, "keyboard-interactive", map[string]any{"jumpHostId": jumpHost.ID}))
	fixture.call("POST", "/api/v1/hosts", hostRequest("嵌套", target.Address(), "x", "password", map[string]any{"jumpHostId": targetHost.ID}), 400)
	fixture.call("DELETE", "/api/v1/hosts/"+jumpHost.ID, nil, 409)
	request := map[string]any{"purpose": "terminal", "hostId": targetHost.ID, "rows": 24, "cols": 80, "credentials": map[string]string{}, "jumpCredentials": map[string]string{"password": sshserver.Password}}
	var seen []string
	wrongCode := func(prompt connections.Prompt) (bool, []string) {
		seen = append(seen, prompt.Hop+":"+prompt.Kind)
		if prompt.Kind == connections.PromptHostKey {
			return true, nil
		}
		if len(prompt.Questions) != 2 || prompt.Questions[0].Echo || prompt.Questions[1].Echo {
			test.Fatalf("challenge questions not relayed: %+v", prompt.Questions)
		}
		return true, []string{sshserver.MFAPassword, "000000"}
	}
	if code := fixture.failure(request, wrongCode); code != "ssh_auth_failed" {
		test.Fatalf("wrong MFA code: %s", code)
	}
	if len(seen) < 3 || seen[0] != "jump:host-key" || seen[1] != "target:host-key" || seen[2] != "target:keyboard-interactive" {
		test.Fatalf("hops verified in wrong order: %v", seen)
	}
	view := fixture.run(request, func(prompt connections.Prompt) (bool, []string) {
		if prompt.Kind != connections.PromptChallenge {
			test.Fatalf("host keys asked again: %+v", prompt)
		}
		return true, []string{sshserver.MFAPassword, sshserver.MFACode}
	})
	if view.State != "connected" || view.Result.Terminal == nil {
		test.Fatalf("MFA through jump failed: %+v %v", view, view.Error)
	}
	terminal := view.Result.Terminal
	base := "/api/v1/terminals/" + terminal.ID
	listing, _ := fixture.call("GET", base+"/files?path=.", nil, 200)
	if !bytes.Contains(listing, []byte("/workspace")) {
		test.Fatalf("SFTP over the shared interactive transport failed: %s", listing)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var cursor uint64
	socket := fixture.connectSocket(ctx, terminal.ID, cursor)
	defer socket.CloseNow()
	readUntil(test, ctx, socket, &cursor, "/workspace $")
	socket.Write(ctx, websocket.MessageBinary, append([]byte{2}, []byte("exit 7\r")...))
	for {
		messageType, data, err := socket.Read(ctx)
		if err != nil {
			test.Fatal("no exit state", err)
		}
		if messageType == websocket.MessageBinary {
			cursor += uint64(len(data) - 9)
			socket.Write(ctx, websocket.MessageText, []byte(`{"type":"ack","offset":"`+strconv.FormatUint(cursor, 10)+`"}`))
			continue
		}
		var state struct {
			Type     string `json:"type"`
			Status   string `json:"status"`
			ExitCode *int   `json:"exitCode"`
		}
		json.Unmarshal(data, &state)
		if state.Type == "state" {
			if state.Status != "exited" || state.ExitCode == nil || *state.ExitCode != 7 {
				test.Fatalf("exit status lost: %s", data)
			}
			break
		}
	}
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		test.Fatal(err)
	}
	defer occupied.Close()
	echoHost, echoPort, _ := net.SplitHostPort(echo.Address())
	targetPort, _ := strconv.Atoi(echoPort)
	conflictBody, _ := fixture.call("POST", "/api/v1/tunnels", map[string]any{"name": "占用", "hostId": jumpHost.ID, "listenHost": "127.0.0.1", "listenPort": occupied.Addr().(*net.TCPAddr).Port, "targetHost": echoHost, "targetPort": targetPort}, 201)
	conflict := decodeInto[tunnels.View](test, conflictBody)
	tunnelRequest := map[string]any{"purpose": "tunnel", "tunnelId": conflict.ID, "credentials": map[string]string{"password": sshserver.Password}}
	refused, _ := fixture.call("POST", "/api/v1/attempts", tunnelRequest, 409)
	if !bytes.Contains(refused, []byte("port_in_use")) {
		test.Fatalf("port conflict not reported: %s", refused)
	}
	fixture.call("POST", "/api/v1/tunnels", map[string]any{"name": "公开", "hostId": jumpHost.ID, "listenHost": "0.0.0.0", "listenPort": 8080, "targetHost": echoHost, "targetPort": targetPort}, 400)
	port := freePort(test)
	createdBody, _ := fixture.call("POST", "/api/v1/tunnels", map[string]any{"name": "回声", "hostId": jumpHost.ID, "listenHost": "127.0.0.1", "listenPort": port, "targetHost": echoHost, "targetPort": targetPort}, 201)
	created := decodeInto[tunnels.View](test, createdBody)
	tunnelRequest["tunnelId"] = created.ID
	started := fixture.run(tunnelRequest, func(prompt connections.Prompt) (bool, []string) {
		test.Fatalf("unexpected prompt %+v", prompt)
		return false, nil
	})
	if started.State != "connected" || started.Result.Tunnel == nil || started.Result.Tunnel.Status.State != tunnels.StateRunning {
		test.Fatalf("tunnel did not start: %+v %v", started, started.Error)
	}
	local, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		test.Fatal(err)
	}
	message := []byte("隧道往返 tunnel round trip")
	local.Write(message)
	reply := make([]byte, len(message))
	local.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(local, reply); err != nil || !bytes.Equal(reply, message) {
		test.Fatalf("tunnel round trip failed: %q %v", reply, err)
	}
	local.Close()
	fixture.call("PUT", "/api/v1/tunnels/"+created.ID, map[string]any{"name": "改名", "hostId": jumpHost.ID, "listenHost": "127.0.0.1", "listenPort": port, "targetHost": echoHost, "targetPort": targetPort}, 409)
	fixture.call("DELETE", "/api/v1/hosts/"+jumpHost.ID, nil, 409)
	fixture.call("POST", "/api/v1/tunnels/"+created.ID+"/stop", nil, 204)
	listed, _ := fixture.call("GET", "/api/v1/tunnels", nil, 200)
	for _, view := range decodeInto[[]tunnels.View](test, listed) {
		if view.ID == created.ID && view.Status.State != tunnels.StateStopped {
			test.Fatalf("tunnel still running after stop: %+v", view.Status)
		}
	}
	if connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second); err == nil {
		connection.Close()
		test.Fatal("stopped tunnel still listening")
	}
	restarted := fixture.run(tunnelRequest, func(connections.Prompt) (bool, []string) { return false, nil })
	if restarted.State != "connected" {
		test.Fatalf("tunnel restart failed: %+v", restarted.Error)
	}
	fixture.call("POST", "/api/v1/auth/logout", nil, 204)
	deadline := time.Now().Add(3 * time.Second)
	for {
		connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
		if err != nil {
			break
		}
		connection.Close()
		if time.Now().After(deadline) {
			test.Fatal("logout left the tunnel listening")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestLocalIdentityFileFollowsSymlinkAndCustomPaths(test *testing.T) {
	fixture := newHarness(test)
	remote := startFixture(test)
	fixture.authorize()
	home := os.Getenv("HOME")
	directory := filepath.Join(home, ".ssh")
	os.MkdirAll(directory, 0700)
	key, err := remote.EncryptedClientKeyPEM("口令 passphrase")
	if err != nil {
		test.Fatal(err)
	}
	os.WriteFile(filepath.Join(directory, "id_fixture"), key, 0600)
	outside := filepath.Join(test.TempDir(), "outside")
	os.WriteFile(outside, remote.ClientKeyPEM(), 0600)
	os.Symlink(outside, filepath.Join(directory, "linked"))
	fixture.call("POST", "/api/v1/hosts", hostRequest("坏路径", remote.Address(), sshserver.KeyUsername, "key", map[string]any{"identityFile": "~/.ssh/../outside"}), 400)
	good := fixture.createHost(hostRequest("本机私钥", remote.Address(), sshserver.KeyUsername, "key", map[string]any{"identityFile": "~/.ssh/id_fixture"}))
	linked := fixture.createHost(hostRequest("软链接私钥", remote.Address(), sshserver.KeyUsername, "key", map[string]any{"identityFile": "~/.ssh/linked"}))
	request := map[string]any{"purpose": "terminal", "hostId": good.ID, "rows": 24, "cols": 80, "credentials": map[string]string{}}
	// Key problems are reported before any network handshake.
	if code := fixture.failure(request, trusting(fixture)); code != "passphrase_required" {
		test.Fatalf("encrypted key without passphrase: %s", code)
	}
	request["credentials"] = map[string]string{"passphrase": "口令 passphrase"}
	terminal := fixture.open(request, "target")
	fixture.call("DELETE", "/api/v1/terminals/"+terminal.ID, nil, 204)
	// Symlinked key pointing outside ~/.ssh now works cleanly.
	linkedReq := map[string]any{"purpose": "terminal", "hostId": linked.ID, "rows": 24, "cols": 80, "credentials": map[string]string{}}
	linkedTerm := fixture.open(linkedReq, "target")
	fixture.call("DELETE", "/api/v1/terminals/"+linkedTerm.ID, nil, 204)
}

func TestVaultSavedCredentialsWorkspaceAndSnippets(test *testing.T) {
	fixture := newHarness(test)
	remote := startFixture(test)
	fixture.authorize()
	host := fixture.createHost(hostRequest("保存密码", remote.Address(), sshserver.Username, "password", nil))
	fixture.call("POST", "/api/v1/credentials", map[string]any{"label": "x", "credentials": map[string]string{"password": "p"}}, 409)
	fixture.call("POST", "/api/v1/vault/setup", map[string]any{"mode": "password", "password": "short"}, 400)
	status, _ := fixture.call("POST", "/api/v1/vault/setup", map[string]any{"mode": "password", "password": "长期主密码 master password"}, 200)
	if !bytes.Contains(status, []byte(`"unlocked":true`)) {
		test.Fatalf("vault not unlocked after setup: %s", status)
	}
	terminal := fixture.open(map[string]any{"purpose": "terminal", "hostId": host.ID, "rows": 24, "cols": 80, "credentials": map[string]string{"password": sshserver.Password}, "rememberLabel": "测试密码"}, "target")
	fixture.call("DELETE", "/api/v1/terminals/"+terminal.ID, nil, 204)
	listed, _ := fixture.call("GET", "/api/v1/credentials", nil, 200)
	records := decodeInto[[]credentials.Record](test, listed)
	if len(records) != 1 || records[0].Label != "测试密码" || bytes.Contains(listed, []byte(sshserver.Password)) {
		test.Fatalf("credential not remembered safely: %s", listed)
	}
	saved, _ := fixture.call("GET", "/api/v1/hosts", nil, 200)
	if !bytes.Contains(saved, []byte(records[0].ID)) {
		test.Fatal("credential not linked to host")
	}
	useSaved := map[string]any{"purpose": "terminal", "hostId": host.ID, "rows": 24, "cols": 80, "credentials": map[string]string{}, "useSaved": true}
	reused := fixture.open(useSaved)
	fixture.call("DELETE", "/api/v1/terminals/"+reused.ID, nil, 204)
	fixture.call("POST", "/api/v1/vault/lock", nil, 200)
	fixture.call("POST", "/api/v1/attempts", useSaved, 423)
	fixture.call("POST", "/api/v1/vault/unlock", map[string]any{"password": "wrong password!!"}, 401)
	fixture.call("POST", "/api/v1/vault/unlock", map[string]any{"password": "长期主密码 master password"}, 200)
	fixture.call("POST", "/api/v1/vault/reset", map[string]any{"confirm": "yes"}, 400)
	fixture.call("POST", "/api/v1/vault/reset", map[string]any{"confirm": "DELETE ALL SAVED CREDENTIALS"}, 200)
	listed, _ = fixture.call("GET", "/api/v1/credentials", nil, 200)
	saved, _ = fixture.call("GET", "/api/v1/hosts", nil, 200)
	if string(bytes.TrimSpace(listed)) != "[]" || bytes.Contains(saved, []byte(records[0].ID)) {
		test.Fatalf("reset left credentials behind: %s %s", listed, saved)
	}
	workspace, _ := fixture.call("GET", "/api/v1/workspace", nil, 200)
	if !bytes.Contains(workspace, []byte(`"split":"single"`)) {
		test.Fatalf("default workspace missing: %s", workspace)
	}
	layout := map[string]any{"layout": map[string]any{"split": "grid", "tabs": []map[string]string{{"hostId": host.ID}}, "sizes": []float64{50, 50}}, "preferences": map[string]any{"fontSize": 15, "scrollback": 8000, "cursorStyle": "block", "keybindings": map[string]string{"tab.next": "$mod+Shift+BracketRight"}}}
	fixture.call("PUT", "/api/v1/workspace", layout, 200)
	layout["preferences"].(map[string]any)["keybindings"] = map[string]string{"rm -rf": "$mod+K"}
	fixture.call("PUT", "/api/v1/workspace", layout, 400)
	workspace, _ = fixture.call("GET", "/api/v1/workspace", nil, 200)
	if !bytes.Contains(workspace, []byte(`"split":"grid"`)) || !bytes.Contains(workspace, []byte("BracketRight")) {
		test.Fatalf("workspace not persisted: %s", workspace)
	}
	snippetBody, _ := fixture.call("POST", "/api/v1/snippets", map[string]string{"name": "磁盘", "command": "df -h\n", "description": ""}, 201)
	snippet := decodeInto[map[string]any](test, snippetBody)
	if snippet["command"] != "df -h" {
		test.Fatalf("trailing newline not trimmed: %v", snippet)
	}
	fixture.call("PUT", "/api/v1/snippets/"+snippet["id"].(string), map[string]string{"name": "磁盘", "command": "", "description": ""}, 400)
	fixture.call("DELETE", "/api/v1/snippets/"+snippet["id"].(string), nil, 204)
}

func TestFolderUploadChmodArchiveAndRecursiveDelete(test *testing.T) {
	fixture := newHarness(test)
	remote := startFixture(test)
	fixture.authorize()
	host := fixture.createHost(hostRequest("文件", remote.Address(), sshserver.Username, "password", nil))
	terminal := fixture.open(map[string]any{"purpose": "terminal", "hostId": host.ID, "rows": 24, "cols": 80, "credentials": map[string]string{"password": sshserver.Password}}, "target")
	base := "/api/v1/terminals/" + terminal.ID
	upload := func(relative, content string, status int) {
		fixture.call("POST", base+"/upload?"+url.Values{"path": {"/workspace"}, "name": {relative}}.Encode(), []byte(content), status)
	}
	upload("项目/子目录/文件.txt", "深层内容", 201)
	upload("项目/a.txt", "第一层", 201)
	upload("项目/../逃逸.txt", "x", 400)
	upload("项目/a.txt/不可能.txt", "x", 409)
	usage := decodeInto[map[string]any](test, must(fixture.call("GET", base+"/usage?path="+url.QueryEscape("/workspace/项目"), nil, 200)))
	if usage["files"].(float64) != 2 || usage["directories"].(float64) != 1 {
		test.Fatalf("usage wrong: %v", usage)
	}
	// pkg/sftp's in-memory fixture accepts SETSTAT but ignores mode bits, so
	// this checks the request path and validation, not the remote effect.
	fixture.call("POST", base+"/files", map[string]any{"operation": "chmod", "path": "/workspace/项目/a.txt", "mode": "0600"}, 204)
	fixture.call("POST", base+"/files", map[string]any{"operation": "chmod", "path": "/workspace/项目/a.txt", "mode": "999"}, 400)
	fixture.call("POST", base+"/files", map[string]any{"operation": "chmod", "path": "/workspace/不存在", "mode": "0644"}, 404)
	archive, headers := fixture.call("GET", base+"/archive?path="+url.QueryEscape("/workspace/项目"), nil, 200)
	if !strings.Contains(headers.Get("Content-Disposition"), ".zip") {
		test.Fatalf("archive headers: %v", headers)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		test.Fatal(err)
	}
	found := false
	for _, file := range reader.File {
		if file.Name == "项目/子目录/文件.txt" {
			content, _ := file.Open()
			data, _ := io.ReadAll(content)
			found = string(data) == "深层内容"
		}
	}
	if !found {
		test.Fatal("archive missing nested file")
	}
	fixture.call("POST", base+"/files", map[string]any{"operation": "delete", "path": "/workspace/项目", "directory": true, "recursive": true, "expected": 2}, 409)
	fixture.call("POST", base+"/files", map[string]any{"operation": "delete", "path": "/workspace/项目", "directory": true, "recursive": true, "expected": 3}, 204)
	fixture.call("POST", base+"/files", map[string]any{"operation": "delete", "path": "/", "directory": true, "recursive": true, "expected": 0}, 400)
	listing, _ := fixture.call("GET", base+"/files?path=/workspace", nil, 200)
	if bytes.Contains(listing, []byte("项目")) {
		test.Fatal("recursive delete left the directory")
	}
}

func must(body []byte, _ any) []byte { return body }

func TestSSHConfigImportAndLocalDocuments(test *testing.T) {
	fixture := newHarness(test)
	fixture.authorize()
	config := strings.Join([]string{
		"Host bastion",
		"  HostName 127.0.0.1",
		"  Port 2201",
		"  User ops",
		"Host app",
		"  HostName 10.0.0.5",
		"  User deploy",
		"  ProxyJump bastion",
		"Host legacy",
		"  HostName 10.0.0.9",
		"  User old",
		"  ProxyCommand nc %h %p",
		"Host *.internal",
		"  User nobody",
		"Match exec \"touch /tmp/sshwb-should-not-exist\"",
		"  User x",
	}, "\n")
	preview, _ := fixture.call("POST", "/api/v1/imports/ssh-config/preview", map[string]string{"source": "text", "content": config}, 200)
	if !bytes.Contains(preview, []byte(`"alias":"app"`)) || bytes.Contains(preview, []byte(`"alias":"*.internal"`)) || !bytes.Contains(preview, []byte(`"proxyCommand":"nc %h %p"`)) {
		test.Fatalf("unexpected preview: %s", preview)
	}
	if _, err := os.Stat("/tmp/sshwb-should-not-exist"); err == nil {
		test.Fatal("Match exec was executed")
	}
	applied, _ := fixture.call("POST", "/api/v1/imports/ssh-config/apply", map[string]any{"source": "text", "content": config, "aliases": []string{"app"}}, 200)
	result := decodeInto[struct {
		Created []hosts.Profile `json:"created"`
	}](test, applied)
	if len(result.Created) != 2 {
		test.Fatalf("jump alias not imported with target: %s", applied)
	}
	var bastion, app hosts.Profile
	for _, profile := range result.Created {
		switch profile.Name {
		case "bastion":
			bastion = profile
		case "app":
			app = profile
		}
	}
	if app.JumpHostID == "" || app.JumpHostID != bastion.ID || bastion.Port != 2201 || app.Username != "deploy" {
		test.Fatalf("imported profiles wrong: %+v %+v", bastion, app)
	}
	again, _ := fixture.call("POST", "/api/v1/imports/ssh-config/apply", map[string]any{"source": "text", "content": config, "aliases": []string{"app"}}, 200)
	if !bytes.Contains(again, []byte("已存在")) {
		test.Fatalf("duplicate import not skipped: %s", again)
	}
	home := os.Getenv("HOME")
	os.MkdirAll(filepath.Join(home, ".ssh"), 0700)
	os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte("Host local-box\n  HostName 127.0.0.1\n  User me\n  IdentityFile ~/.ssh/id_local\n"), 0600)
	localPreview, _ := fixture.call("POST", "/api/v1/imports/ssh-config/preview", map[string]string{"source": "local"}, 200)
	if !bytes.Contains(localPreview, []byte("local-box")) {
		test.Fatalf("local config not read from HOME: %s", localPreview)
	}
	localApplied, _ := fixture.call("POST", "/api/v1/imports/ssh-config/apply", map[string]any{"source": "local", "aliases": []string{"local-box"}}, 200)
	if !bytes.Contains(localApplied, []byte(`"identityFile":"~/.ssh/id_local"`)) {
		test.Fatalf("identity file not mapped: %s", localApplied)
	}
	document := filepath.Join(test.TempDir(), "说明.md")
	os.WriteFile(document, []byte("# 标题\n"), 0644)
	registration, err := fixture.services.Documents.Register(document)
	if err != nil {
		test.Fatal(err)
	}
	route := "/api/v1/local-documents/" + registration.ID
	loaded := decodeInto[map[string]any](test, must(fixture.call("GET", route, nil, 200)))
	loaded["content"] = "# 新标题\n"
	delete(loaded, "id")
	fixture.call("PUT", route, loaded, 200)
	if data, _ := os.ReadFile(document); string(data) != "# 新标题\n" {
		test.Fatalf("local document not saved: %q", data)
	}
	fixture.call("PUT", route, loaded, 409)
	fixture.call("GET", "/api/v1/local-documents/unknown", nil, 404)
}
