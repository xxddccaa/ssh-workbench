package httpapi

import (
	"io"
	"mime"
	"net/http"
	"path"
	"time"

	"sshworkbench/internal/fault"
	"sshworkbench/internal/files"
	contract "sshworkbench/internal/transport/httpapi/generated"
)

func owner(request *http.Request) string {
	return currentSession(request).ID
}

func (server *Server) listHosts(response http.ResponseWriter, request *http.Request) {
	profiles, err := server.Hosts.List(request.Context())
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, profiles)
}

func (server *Server) saveHost(response http.ResponseWriter, request *http.Request) {
	var input contract.HostInput
	if !decode(response, request, &input, 16*1024) {
		return
	}
	profile := hostInput(input)
	if profile.IdentityFile != "" && !server.config.LocalFiles {
		writeError(response, fault.New(403, "identity_file_disabled", "远程模式不读取服务端本机私钥文件。"))
		return
	}
	saved, err := server.Hosts.Save(request.Context(), request.PathValue("hostID"), profile)
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

func (server *Server) deleteHost(response http.ResponseWriter, request *http.Request) {
	if err := server.Hosts.Delete(request.Context(), request.PathValue("hostID")); err != nil {
		writeError(response, err)
		return
	}
	response.WriteHeader(204)
}

func (server *Server) listTerminals(response http.ResponseWriter, request *http.Request) {
	writeJSON(response, 200, server.Terminals.List(owner(request)))
}

func (server *Server) closeTerminal(response http.ResponseWriter, request *http.Request) {
	if err := server.Terminals.Remove(owner(request), request.PathValue("terminalID")); err != nil {
		writeError(response, err)
		return
	}
	response.WriteHeader(204)
}

func (server *Server) listFiles(response http.ResponseWriter, request *http.Request) {
	listing, err := server.Files.List(request.Context(), owner(request), request.PathValue("terminalID"), request.URL.Query().Get("path"))
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, listing)
}

func (server *Server) mutateFile(response http.ResponseWriter, request *http.Request) {
	var input contract.Mutation
	if !decode(response, request, &input, 16*1024) {
		return
	}
	mutation := files.Mutation{Operation: string(input.Operation), Path: input.Path, Name: valueOf(input.Name), Directory: valueOf(input.Directory), Recursive: valueOf(input.Recursive), Mode: valueOf(input.Mode), Expected: valueOf(input.Expected)}
	if err := server.Files.Mutate(request.Context(), owner(request), request.PathValue("terminalID"), mutation); err != nil {
		writeError(response, err)
		return
	}
	response.WriteHeader(204)
}

func (server *Server) treeUsage(response http.ResponseWriter, request *http.Request) {
	usage, err := server.Files.Inspect(request.Context(), owner(request), request.PathValue("terminalID"), request.URL.Query().Get("path"))
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, usage)
}

func (server *Server) readDocument(response http.ResponseWriter, request *http.Request) {
	document, err := server.Files.ReadDocument(request.Context(), owner(request), request.PathValue("terminalID"), request.URL.Query().Get("path"))
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, document)
}

func (server *Server) saveDocument(response http.ResponseWriter, request *http.Request) {
	var input contract.Document
	if !decode(response, request, &input, files.EditorLimit*6+8192) {
		return
	}
	saved, err := server.Files.SaveDocument(request.Context(), owner(request), request.PathValue("terminalID"), documentInput(input))
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, saved)
}

func (server *Server) upload(response http.ResponseWriter, request *http.Request) {
	http.NewResponseController(response).SetReadDeadline(time.Now().Add(30 * time.Minute))
	request.Body = http.MaxBytesReader(response, request.Body, files.UploadLimit+1)
	if request.ContentLength > files.UploadLimit {
		writeError(response, fault.New(413, "upload_too_large", "单个文件不得超过 1 GiB。"))
		return
	}
	parameters := request.URL.Query()
	err := server.Files.Upload(request.Context(), owner(request), request.PathValue("terminalID"), parameters.Get("path"), parameters.Get("name"), request.Body)
	if err != nil {
		writeError(response, err)
		return
	}
	response.WriteHeader(201)
}

// attachment starts a streamed download only on the first written byte, so
// errors found before streaming still produce a normal JSON error.
type attachment struct {
	response    http.ResponseWriter
	filename    string
	contentType string
	started     bool
}

func (writer *attachment) Write(data []byte) (int, error) {
	if !writer.started {
		writer.response.Header().Set("Content-Type", writer.contentType)
		writer.response.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": writer.filename}))
		writer.started = true
	}
	return writer.response.Write(data)
}

func (server *Server) finishAttachment(response http.ResponseWriter, writer *attachment, err error) {
	if err != nil {
		if writer.started {
			panic(http.ErrAbortHandler)
		}
		writeError(response, err)
		return
	}
	if !writer.started {
		writer.Write(nil)
	}
}

func (server *Server) download(response http.ResponseWriter, request *http.Request) {
	http.NewResponseController(response).SetWriteDeadline(time.Now().Add(30 * time.Minute))
	filename := request.URL.Query().Get("path")
	writer := &attachment{response: response, filename: path.Base(filename), contentType: "application/octet-stream"}
	err := server.Files.Download(request.Context(), owner(request), request.PathValue("terminalID"), filename, writer)
	server.finishAttachment(response, writer, err)
}

func (server *Server) archive(response http.ResponseWriter, request *http.Request) {
	http.NewResponseController(response).SetWriteDeadline(time.Now().Add(2 * time.Hour))
	writer := &attachment{response: response, contentType: "application/zip"}
	err := server.Files.Archive(request.Context(), owner(request), request.PathValue("terminalID"), request.URL.Query().Get("path"), func(filename string) io.Writer {
		writer.filename = filename
		return writer
	})
	server.finishAttachment(response, writer, err)
}
