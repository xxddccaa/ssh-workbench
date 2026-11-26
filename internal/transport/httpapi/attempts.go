package httpapi

import (
	"net/http"
	"strconv"

	"sshworkbench/internal/connections"
	"sshworkbench/internal/fault"
	"sshworkbench/internal/launch"
	contract "sshworkbench/internal/transport/httpapi/generated"
)

func (server *Server) startAttempt(response http.ResponseWriter, request *http.Request) {
	var input contract.AttemptRequest
	if !decode(response, request, &input, 1024*1024) {
		return
	}
	launchRequest := launch.Request{HostID: valueOf(input.HostId), Rows: valueOf(input.Rows), Cols: valueOf(input.Cols), Credentials: credentialInput(input.Credentials), UseSaved: valueOf(input.UseSaved), RememberLabel: valueOf(input.RememberLabel)}
	if input.QuickConnect != nil {
		launchRequest.QuickConnect = &launch.QuickConnect{
			Name:         valueOf(input.QuickConnect.Name),
			Hostname:     input.QuickConnect.Hostname,
			Port:         valueOf(input.QuickConnect.Port),
			Username:     valueOf(input.QuickConnect.Username),
			AuthMode:     string(valueOf(input.QuickConnect.AuthMode)),
			IdentityFile: valueOf(input.QuickConnect.IdentityFile),
			ProxyCommand: valueOf(input.QuickConnect.ProxyCommand),
		}
	}
	if input.JumpCredentials != nil {
		launchRequest.JumpCredentials = credentialInput(*input.JumpCredentials)
	}
	var attempt connections.Attempt
	var err error
	switch input.Purpose {
	case "terminal":
		attempt, err = server.Launch.Terminal(request.Context(), owner(request), launchRequest)
	case "tunnel":
		attempt, err = server.Launch.Tunnel(request.Context(), owner(request), valueOf(input.TunnelId), launchRequest)
	default:
		err = fault.New(400, "invalid_purpose", "连接用途无效。")
	}
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 202, attempt)
}

func (server *Server) waitAttempt(response http.ResponseWriter, request *http.Request) {
	var after int64
	if value := request.URL.Query().Get("after"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			writeError(response, fault.New(400, "invalid_version", "版本参数无效。"))
			return
		}
		after = parsed
	}
	attempt, err := server.Attempts.Wait(request.Context(), owner(request), request.PathValue("attemptID"), after)
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, attempt)
}

func (server *Server) answerAttempt(response http.ResponseWriter, request *http.Request) {
	var input contract.AttemptAnswer
	if !decode(response, request, &input, 64*1024) {
		return
	}
	answer := connections.Answer{PromptID: input.PromptId, Accept: input.Accept, Responses: valueOf(input.Responses)}
	if err := server.Attempts.Answer(owner(request), request.PathValue("attemptID"), answer); err != nil {
		writeError(response, err)
		return
	}
	response.WriteHeader(204)
}

func (server *Server) cancelAttempt(response http.ResponseWriter, request *http.Request) {
	if err := server.Attempts.Cancel(owner(request), request.PathValue("attemptID")); err != nil {
		writeError(response, err)
		return
	}
	response.WriteHeader(204)
}
