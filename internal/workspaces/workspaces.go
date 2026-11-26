// Package workspaces persists the workbench layout and preferences. It
// stores host references and arrangement only, never live SSH objects: after
// a reload the layout comes back with disconnected placeholders.
package workspaces

import (
	"context"
	"encoding/json"
	"regexp"
	"time"

	"sshworkbench/internal/fault"
)

const setting = "workspace"

var (
	actions = map[string]bool{
		"hosts.search": true, "connect.quick": true, "tab.next": true, "tab.previous": true, "tab.close": true,
		"pane.next": true, "layout.single": true, "layout.columns": true, "layout.rows": true, "layout.grid": true,
		"files.toggle": true, "transfers.toggle": true, "tunnels.toggle": true, "terminal.search": true,
		"snippets.open": true, "broadcast.open": true, "font.larger": true, "font.smaller": true,
	}
	combination = regexp.MustCompile(`^(\$mod\+|Control\+|Meta\+|Alt\+|Shift\+){1,3}[A-Za-z0-9]{1,12}$`)
	layouts     = map[string]bool{"single": true, "columns": true, "rows": true, "grid": true}
)

type Tab struct {
	HostID string `json:"hostId"`
}

type Layout struct {
	Split string    `json:"split"`
	Tabs  []Tab     `json:"tabs"`
	Sizes []float64 `json:"sizes"`
}

type Preferences struct {
	FontSize    int               `json:"fontSize"`
	Scrollback  int               `json:"scrollback"`
	CursorStyle string            `json:"cursorStyle"`
	Keybindings map[string]string `json:"keybindings"`
}

type Workspace struct {
	Layout      Layout      `json:"layout"`
	Preferences Preferences `json:"preferences"`
	UpdatedAt   time.Time   `json:"updatedAt"`
}

type Settings interface {
	GetSetting(context.Context, string) (string, error)
	PutSetting(context.Context, string, string) error
}

type Service struct {
	settings Settings
}

func New(settings Settings) *Service {
	return &Service{settings: settings}
}

func Default() Workspace {
	return Workspace{Layout: Layout{Split: "single", Tabs: []Tab{}, Sizes: []float64{}}, Preferences: Preferences{FontSize: 14, Scrollback: 5000, CursorStyle: "bar", Keybindings: map[string]string{}}}
}

func (service *Service) Get(ctx context.Context) (Workspace, error) {
	value, err := service.settings.GetSetting(ctx, setting)
	if err != nil || value == "" {
		return Default(), err
	}
	var workspace Workspace
	if json.Unmarshal([]byte(value), &workspace) != nil || Validate(workspace) != nil {
		return Default(), nil
	}
	return workspace, nil
}

func Validate(workspace Workspace) error {
	invalid := fault.New(400, "invalid_workspace", "工作区布局或偏好设置无效。")
	if !layouts[workspace.Layout.Split] || len(workspace.Layout.Tabs) > 8 || len(workspace.Layout.Sizes) > 8 {
		return invalid
	}
	for _, tab := range workspace.Layout.Tabs {
		if tab.HostID == "" || len(tab.HostID) > 260 {
			return invalid
		}
	}
	for _, size := range workspace.Layout.Sizes {
		if size < 5 || size > 95 {
			return invalid
		}
	}
	preferences := workspace.Preferences
	if preferences.FontSize < 10 || preferences.FontSize > 28 || preferences.Scrollback < 500 || preferences.Scrollback > 50000 {
		return invalid
	}
	if preferences.CursorStyle != "bar" && preferences.CursorStyle != "block" && preferences.CursorStyle != "underline" {
		return invalid
	}
	if len(preferences.Keybindings) > len(actions) {
		return invalid
	}
	for action, keys := range preferences.Keybindings {
		if !actions[action] || (keys != "" && !combination.MatchString(keys)) {
			return fault.New(400, "invalid_keybinding", "快捷键设置无效："+action)
		}
	}
	return nil
}

func (service *Service) Put(ctx context.Context, workspace Workspace) (Workspace, error) {
	if workspace.Preferences.Keybindings == nil {
		workspace.Preferences.Keybindings = map[string]string{}
	}
	if workspace.Layout.Tabs == nil {
		workspace.Layout.Tabs = []Tab{}
	}
	if workspace.Layout.Sizes == nil {
		workspace.Layout.Sizes = []float64{}
	}
	if err := Validate(workspace); err != nil {
		return Workspace{}, err
	}
	workspace.UpdatedAt = time.Now().UTC()
	document, err := json.Marshal(workspace)
	if err != nil {
		return Workspace{}, err
	}
	if err := service.settings.PutSetting(ctx, setting, string(document)); err != nil {
		return Workspace{}, err
	}
	return workspace, nil
}
