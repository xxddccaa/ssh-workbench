package httpapi

import (
	"sshworkbench/internal/connections"
	"sshworkbench/internal/files"
	"sshworkbench/internal/hosts"
	contract "sshworkbench/internal/transport/httpapi/generated"
)

func valueOf[Value any](pointer *Value) Value {
	if pointer != nil {
		return *pointer
	}
	var zero Value
	return zero
}

func hostInput(input contract.HostInput) hosts.Profile {
	return hosts.Profile{Name: input.Name, Hostname: input.Hostname, Port: input.Port, Username: input.Username, AuthMode: string(input.AuthMode), Group: input.Group, Color: string(input.Color), Favorite: input.Favorite, Production: valueOf(input.Production), JumpHostID: valueOf(input.JumpHostId), ProxyCommand: valueOf(input.ProxyCommand), IdentityFile: valueOf(input.IdentityFile), CredentialID: valueOf(input.CredentialId)}
}

func credentialInput(input contract.Credentials) connections.Credentials {
	return connections.Credentials{Password: valueOf(input.Password), PrivateKey: valueOf(input.PrivateKey), Passphrase: valueOf(input.Passphrase)}
}

func documentInput(input contract.Document) files.Document {
	return files.Document{Path: input.Path, Content: input.Content, Version: input.Version, LineEnding: string(input.LineEnding), BOM: input.Bom}
}
