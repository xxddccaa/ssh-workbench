package workspaces

import "testing"

func TestWorkspaceValidation(test *testing.T) {
	valid := Default()
	valid.Layout = Layout{Split: "grid", Tabs: []Tab{{HostID: "a"}}, Sizes: []float64{40, 60}}
	valid.Preferences.Keybindings = map[string]string{"tab.next": "Alt+Shift+ArrowRight", "files.toggle": ""}
	if err := Validate(valid); err != nil {
		test.Fatal(err)
	}
	for name, mutate := range map[string]func(*Workspace){
		"unknown split":  func(workspace *Workspace) { workspace.Layout.Split = "mosaic" },
		"too many tabs":  func(workspace *Workspace) { workspace.Layout.Tabs = make([]Tab, 9) },
		"empty host":     func(workspace *Workspace) { workspace.Layout.Tabs = []Tab{{}} },
		"tiny pane":      func(workspace *Workspace) { workspace.Layout.Sizes = []float64{1, 99} },
		"font":           func(workspace *Workspace) { workspace.Preferences.FontSize = 72 },
		"scrollback":     func(workspace *Workspace) { workspace.Preferences.Scrollback = 10 },
		"cursor":         func(workspace *Workspace) { workspace.Preferences.CursorStyle = "beam" },
		"unknown action": func(workspace *Workspace) { workspace.Preferences.Keybindings = map[string]string{"rm": "Alt+KeyR"} },
		"malformed combo": func(workspace *Workspace) {
			workspace.Preferences.Keybindings = map[string]string{"tab.next": "<script>"}
		},
	} {
		candidate := valid
		candidate.Layout.Tabs = append([]Tab(nil), valid.Layout.Tabs...)
		candidate.Preferences.Keybindings = map[string]string{}
		mutate(&candidate)
		if Validate(candidate) == nil {
			test.Errorf("%s accepted", name)
		}
	}
}
