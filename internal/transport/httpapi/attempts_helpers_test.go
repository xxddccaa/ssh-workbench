package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"sshworkbench/internal/connections"
	"sshworkbench/internal/fault"
	"sshworkbench/internal/terminals"
	"sshworkbench/internal/tunnels"
)

type attemptView struct {
	ID      string              `json:"id"`
	State   string              `json:"state"`
	Version int64               `json:"version"`
	Prompt  *connections.Prompt `json:"prompt"`
	Error   *fault.Error        `json:"error"`
	Result  struct {
		Terminal *terminals.Info `json:"terminal"`
		Tunnel   *tunnels.View   `json:"tunnel"`
		Warning  string          `json:"warning"`
	} `json:"result"`
}

type responder func(connections.Prompt) (bool, []string)

// run starts an attempt and answers every prompt with respond until the
// attempt connects or fails.
func (fixture *harness) run(request map[string]any, respond responder) attemptView {
	fixture.test.Helper()
	body, _ := fixture.call("POST", "/api/v1/attempts", request, 202)
	var view attemptView
	if err := json.Unmarshal(body, &view); err != nil {
		fixture.test.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		switch view.State {
		case "connected", "failed", "cancelled":
			return view
		case "prompt":
			accept, responses := respond(*view.Prompt)
			fixture.call("POST", "/api/v1/attempts/"+view.ID+"/answer", map[string]any{"promptId": view.Prompt.ID, "accept": accept, "responses": responses}, 204)
		}
		body, _ = fixture.call("GET", "/api/v1/attempts/"+view.ID+"?"+url.Values{"after": {strconv.FormatInt(view.Version, 10)}}.Encode(), nil, 200)
		view = attemptView{}
		if err := json.Unmarshal(body, &view); err != nil {
			fixture.test.Fatal(err)
		}
	}
	fixture.test.Fatal("attempt did not finish")
	return view
}

// trusting accepts host keys for the listed hops, in order, and fails the
// test on any other prompt.
func trusting(fixture *harness, hops ...string) responder {
	return func(prompt connections.Prompt) (bool, []string) {
		if prompt.Kind != connections.PromptHostKey || len(hops) == 0 || prompt.Hop != hops[0] || prompt.Fingerprint == "" {
			fixture.test.Fatalf("unexpected prompt %+v (expected host key for %v)", prompt, hops)
		}
		hops = hops[1:]
		return true, nil
	}
}

func (fixture *harness) open(request map[string]any, trustHops ...string) terminals.Info {
	fixture.test.Helper()
	view := fixture.run(request, trusting(fixture, trustHops...))
	if view.State != "connected" || view.Result.Terminal == nil {
		fixture.test.Fatalf("terminal not opened: %+v %v", view, view.Error)
	}
	return *view.Result.Terminal
}

func (fixture *harness) failure(request map[string]any, respond responder) string {
	fixture.test.Helper()
	view := fixture.run(request, respond)
	if view.State != "failed" || view.Error == nil {
		fixture.test.Fatalf("attempt did not fail: %+v", view)
	}
	return view.Error.Code
}

func hostRequest(name, address, username, authMode string, extra map[string]any) map[string]any {
	host, portValue, _ := splitAddress(address)
	request := map[string]any{"name": name, "hostname": host, "port": portValue, "username": username, "authMode": authMode, "color": "teal", "group": "", "favorite": false}
	for key, value := range extra {
		request[key] = value
	}
	return request
}

func splitAddress(address string) (string, int, error) {
	var host string
	var port int
	_, err := fmt.Sscanf(address, "127.0.0.1:%d", &port)
	host = "127.0.0.1"
	return host, port, err
}
