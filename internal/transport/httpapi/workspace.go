package httpapi

import (
	"net/http"

	"sshworkbench/internal/fault"
	"sshworkbench/internal/imports"
	"sshworkbench/internal/snippets"
	contract "sshworkbench/internal/transport/httpapi/generated"
	"sshworkbench/internal/tunnels"
	"sshworkbench/internal/workspaces"
)

var invalidConfirmation = fault.Error{Status: 400, Code: "confirmation_required", Message: "请输入确认文字。"}

func (server *Server) listTunnels(response http.ResponseWriter, request *http.Request) {
	views, err := server.Tunnels.List(request.Context())
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, views)
}

func (server *Server) saveTunnel(response http.ResponseWriter, request *http.Request) {
	var input contract.TunnelInput
	if !decode(response, request, &input, 4096) {
		return
	}
	if _, err := server.Hosts.Get(request.Context(), input.HostId); err != nil {
		writeError(response, err)
		return
	}
	spec, err := server.Tunnels.Save(request.Context(), request.PathValue("tunnelID"), tunnels.Spec{Name: input.Name, HostID: input.HostId, ListenHost: string(input.ListenHost), ListenPort: input.ListenPort, TargetHost: input.TargetHost, TargetPort: input.TargetPort})
	if err != nil {
		writeError(response, err)
		return
	}
	status := 200
	if request.Method == http.MethodPost {
		status = 201
	}
	writeJSON(response, status, tunnels.View{Spec: spec, Status: tunnels.Status{State: tunnels.StateStopped}})
}

func (server *Server) deleteTunnel(response http.ResponseWriter, request *http.Request) {
	if err := server.Tunnels.Delete(request.Context(), request.PathValue("tunnelID")); err != nil {
		writeError(response, err)
		return
	}
	response.WriteHeader(204)
}

func (server *Server) stopTunnel(response http.ResponseWriter, request *http.Request) {
	if err := server.Tunnels.Stop(request.PathValue("tunnelID")); err != nil {
		writeError(response, err)
		return
	}
	response.WriteHeader(204)
}

func (server *Server) listSnippets(response http.ResponseWriter, request *http.Request) {
	items, err := server.Snippets.List(request.Context())
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, items)
}

func (server *Server) saveSnippet(response http.ResponseWriter, request *http.Request) {
	var input contract.SnippetInput
	if !decode(response, request, &input, 32*1024) {
		return
	}
	saved, err := server.Snippets.Save(request.Context(), request.PathValue("snippetID"), snippets.Snippet{Name: input.Name, Command: input.Command, Description: input.Description})
	if err != nil {
		writeError(response, err)
		return
	}
	status := 200
	if request.Method == http.MethodPost {
		status = 201
	}
	writeJSON(response, status, saved)
}

func (server *Server) deleteSnippet(response http.ResponseWriter, request *http.Request) {
	if err := server.Snippets.Delete(request.Context(), request.PathValue("snippetID")); err != nil {
		writeError(response, err)
		return
	}
	response.WriteHeader(204)
}

func (server *Server) getWorkspace(response http.ResponseWriter, request *http.Request) {
	workspace, err := server.Workspaces.Get(request.Context())
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, workspace)
}

func (server *Server) putWorkspace(response http.ResponseWriter, request *http.Request) {
	var input contract.Workspace
	if !decode(response, request, &input, 64*1024) {
		return
	}
	saved, err := server.Workspaces.Put(request.Context(), workspaceInput(input))
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, saved)
}

func workspaceInput(input contract.Workspace) workspaces.Workspace {
	workspace := workspaces.Workspace{Layout: workspaces.Layout{Split: string(input.Layout.Split), Tabs: make([]workspaces.Tab, 0, len(input.Layout.Tabs)), Sizes: make([]float64, 0, len(input.Layout.Sizes))}, Preferences: workspaces.Preferences{FontSize: input.Preferences.FontSize, Scrollback: input.Preferences.Scrollback, CursorStyle: string(input.Preferences.CursorStyle), Keybindings: input.Preferences.Keybindings}}
	for _, tab := range input.Layout.Tabs {
		workspace.Layout.Tabs = append(workspace.Layout.Tabs, workspaces.Tab{HostID: tab.HostId})
	}
	for _, size := range input.Layout.Sizes {
		workspace.Layout.Sizes = append(workspace.Layout.Sizes, float64(size))
	}
	return workspace
}

func (server *Server) listSSHConfigHosts(response http.ResponseWriter, request *http.Request) {
	if !server.config.LocalFiles {
		writeJSON(response, 200, imports.SSHConfigHosts{Hosts: []imports.SSHConfigHost{}})
		return
	}
	result, err := server.Imports.ListHosts()
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, result)
}

func (server *Server) importSource(source, content string) (imports.Source, error) {
	switch source {
	case "local":
		if !server.config.LocalFiles {
			return imports.Source{}, fault.New(403, "local_import_disabled", "远程模式不读取服务端的 ~/.ssh/config，请粘贴或上传配置内容。")
		}
		return imports.Source{Local: true}, nil
	case "text":
		return imports.Source{Content: []byte(content)}, nil
	}
	return imports.Source{}, fault.New(400, "invalid_import_source", "导入来源无效。")
}

func (server *Server) previewImport(response http.ResponseWriter, request *http.Request) {
	var input contract.ImportSource
	if !decode(response, request, &input, 2*1024*1024) {
		return
	}
	source, err := server.importSource(string(input.Source), valueOf(input.Content))
	if err != nil {
		writeError(response, err)
		return
	}
	preview, err := server.Imports.Preview(request.Context(), source)
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, preview)
}

func (server *Server) applyImport(response http.ResponseWriter, request *http.Request) {
	var input contract.ImportApply
	if !decode(response, request, &input, 2*1024*1024) {
		return
	}
	source, err := server.importSource(string(input.Source), valueOf(input.Content))
	if err != nil {
		writeError(response, err)
		return
	}
	result, err := server.Imports.Apply(request.Context(), source, input.Aliases, valueOf(input.Group))
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, result)
}

func (server *Server) localDocumentsEnabled(response http.ResponseWriter) bool {
	if !server.config.LocalFiles || server.Documents == nil {
		writeError(response, fault.New(403, "local_documents_disabled", "远程模式不提供本机文件编辑。"))
		return false
	}
	return true
}

func (server *Server) readLocalDocument(response http.ResponseWriter, request *http.Request) {
	if !server.localDocumentsEnabled(response) {
		return
	}
	identifier := request.PathValue("documentID")
	document, err := server.Documents.Read(identifier)
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, map[string]any{"id": identifier, "path": document.Path, "content": document.Content, "version": document.Version, "lineEnding": document.LineEnding, "bom": document.BOM})
}

func (server *Server) saveLocalDocument(response http.ResponseWriter, request *http.Request) {
	if !server.localDocumentsEnabled(response) {
		return
	}
	var input contract.Document
	if !decode(response, request, &input, 2*1024*1024*6+8192) {
		return
	}
	identifier := request.PathValue("documentID")
	document, err := server.Documents.Save(identifier, documentInput(input))
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, map[string]any{"id": identifier, "path": document.Path, "content": document.Content, "version": document.Version, "lineEnding": document.LineEnding, "bom": document.BOM})
}
