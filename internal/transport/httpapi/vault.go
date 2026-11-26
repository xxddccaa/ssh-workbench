package httpapi

import (
	"net/http"

	contract "sshworkbench/internal/transport/httpapi/generated"
)

func (server *Server) writeVaultStatus(response http.ResponseWriter, request *http.Request) {
	status, err := server.Vault.Status(request.Context())
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, status)
}

func (server *Server) vaultStatus(response http.ResponseWriter, request *http.Request) {
	server.writeVaultStatus(response, request)
}

func (server *Server) setupVault(response http.ResponseWriter, request *http.Request) {
	var input contract.VaultSetup
	if !decode(response, request, &input, 4096) {
		return
	}
	if err := server.Vault.Setup(request.Context(), string(input.Mode), valueOf(input.Password)); err != nil {
		writeError(response, err)
		return
	}
	server.writeVaultStatus(response, request)
}

func (server *Server) unlockVault(response http.ResponseWriter, request *http.Request) {
	var input contract.VaultUnlock
	if !decode(response, request, &input, 4096) {
		return
	}
	if err := server.Vault.Unlock(request.Context(), valueOf(input.Password)); err != nil {
		writeError(response, err)
		return
	}
	server.writeVaultStatus(response, request)
}

func (server *Server) lockVault(response http.ResponseWriter, request *http.Request) {
	server.Vault.Lock()
	server.writeVaultStatus(response, request)
}

func (server *Server) resetVault(response http.ResponseWriter, request *http.Request) {
	var input contract.VaultReset
	if !decode(response, request, &input, 1024) {
		return
	}
	if input.Confirm != "DELETE ALL SAVED CREDENTIALS" {
		writeError(response, &invalidConfirmation)
		return
	}
	if err := server.Credentials.Reset(request.Context()); err != nil {
		writeError(response, err)
		return
	}
	server.writeVaultStatus(response, request)
}

func (server *Server) listCredentials(response http.ResponseWriter, request *http.Request) {
	records, err := server.Credentials.List(request.Context())
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 200, records)
}

func (server *Server) createCredential(response http.ResponseWriter, request *http.Request) {
	var input contract.CredentialInput
	if !decode(response, request, &input, 512*1024) {
		return
	}
	record, err := server.Credentials.Create(request.Context(), input.Label, credentialInput(input.Credentials))
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, 201, record)
}

func (server *Server) deleteCredential(response http.ResponseWriter, request *http.Request) {
	if err := server.Credentials.Delete(request.Context(), request.PathValue("credentialID")); err != nil {
		writeError(response, err)
		return
	}
	response.WriteHeader(204)
}
