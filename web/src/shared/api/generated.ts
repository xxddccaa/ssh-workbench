export interface paths {
    "/api/v1/attempts": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["startAttempt"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/attempts/{attemptID}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                attemptID: string;
            };
            cookie?: never;
        };
        get: operations["waitAttempt"];
        put?: never;
        post?: never;
        delete: operations["cancelAttempt"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/attempts/{attemptID}/answer": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                attemptID: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["answerAttempt"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/auth": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["authState"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/auth/bootstrap": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["bootstrap"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/auth/login": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["login"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/auth/logout": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["logout"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/credentials": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listCredentials"];
        put?: never;
        post: operations["createCredential"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/credentials/{credentialID}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                credentialID: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        delete: operations["deleteCredential"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/hosts": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listHosts"];
        put?: never;
        post: operations["createHost"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/hosts/{hostID}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                hostID: string;
            };
            cookie?: never;
        };
        get?: never;
        put: operations["updateHost"];
        post?: never;
        delete: operations["deleteHost"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/imports/ssh-config/apply": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["applySSHConfig"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/imports/ssh-config/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["previewSSHConfig"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/local-documents/{documentID}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                documentID: string;
            };
            cookie?: never;
        };
        get: operations["readLocalDocument"];
        put: operations["saveLocalDocument"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/snippets": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listSnippets"];
        put?: never;
        post: operations["createSnippet"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/snippets/{snippetID}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                snippetID: string;
            };
            cookie?: never;
        };
        get?: never;
        put: operations["updateSnippet"];
        post?: never;
        delete: operations["deleteSnippet"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/ssh-config/hosts": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listSSHConfigHosts"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/terminals": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listTerminals"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/terminals/{terminalID}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        delete: operations["closeTerminal"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/terminals/{terminalID}/archive": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        get: operations["downloadArchive"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/terminals/{terminalID}/document": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        get: operations["readDocument"];
        put: operations["saveDocument"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/terminals/{terminalID}/download": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        get: operations["downloadFile"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/terminals/{terminalID}/files": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        get: operations["listFiles"];
        put?: never;
        post: operations["mutateFile"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/terminals/{terminalID}/upload": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["uploadFile"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/terminals/{terminalID}/usage": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        get: operations["treeUsage"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tunnels": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listTunnels"];
        put?: never;
        post: operations["createTunnel"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tunnels/{tunnelID}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tunnelID: string;
            };
            cookie?: never;
        };
        get?: never;
        put: operations["updateTunnel"];
        post?: never;
        delete: operations["deleteTunnel"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/tunnels/{tunnelID}/stop": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                tunnelID: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["stopTunnel"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/vault": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["vaultStatus"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/vault/lock": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["lockVault"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/vault/reset": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["resetVault"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/vault/setup": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["setupVault"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/vault/unlock": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["unlockVault"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/workspace": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getWorkspace"];
        put: operations["putWorkspace"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
}
export type webhooks = Record<string, never>;
export interface components {
    schemas: {
        Attempt: {
            error?: components["schemas"]["Error"];
            id: string;
            prompt?: components["schemas"]["Prompt"];
            purpose: "terminal" | "tunnel";
            result?: components["schemas"]["LaunchResult"];
            state: "connecting" | "prompt" | "connected" | "failed" | "cancelled";
            version: number;
        };
        AttemptAnswer: {
            accept: boolean;
            promptId: string;
            responses?: string[];
        };
        AttemptRequest: {
            cols?: number;
            credentials: components["schemas"]["Credentials"];
            hostId?: string;
            jumpCredentials?: components["schemas"]["Credentials"];
            purpose: "terminal" | "tunnel";
            quickConnect?: components["schemas"]["QuickConnectTarget"];
            rememberLabel?: string;
            rows?: number;
            tunnelId?: string;
            useSaved?: boolean;
        };
        AuthState: {
            authenticated: boolean;
            authRequired?: boolean;
            capabilities?: {
                agent: boolean;
                localFiles: boolean;
                localTerminal: boolean;
            };
            csrfToken?: string;
            expiresAt?: string;
            remote: boolean;
            version: string;
        };
        BootstrapRequest: {
            token: string;
        };
        CredentialInput: {
            credentials: components["schemas"]["Credentials"];
            label: string;
        };
        CredentialRecord: {
            createdAt: string;
            id: string;
            kind: "password" | "key";
            label: string;
        };
        Credentials: {
            passphrase?: string;
            password?: string;
            privateKey?: string;
        };
        Document: {
            bom: boolean;
            content: string;
            lineEnding: "lf" | "crlf" | "mixed";
            path: string;
            version: string;
        };
        Error: {
            code: string;
            details?: unknown;
            message: string;
        };
        ErrorResponse: {
            error: components["schemas"]["Error"];
        };
        FileEntry: {
            accessible: boolean;
            directory: boolean;
            mode: string;
            modifiedAt: string;
            name: string;
            path: string;
            size: number;
            symlink: boolean;
        };
        FileListing: {
            entries: components["schemas"]["FileEntry"][];
            path: string;
        };
        HostInput: {
            authMode: "auto" | "password" | "key" | "agent" | "keyboard-interactive";
            color: "teal" | "blue" | "amber" | "rose";
            credentialId?: string;
            favorite: boolean;
            group: string;
            hostname: string;
            identityFile?: string;
            jumpHostId?: string;
            name: string;
            port: number;
            production?: boolean;
            proxyCommand?: string;
            username: string;
        };
        HostProfile: {
            authMode: "auto" | "password" | "key" | "agent" | "keyboard-interactive";
            color: "teal" | "blue" | "amber" | "rose";
            createdAt: string;
            credentialId?: string;
            favorite: boolean;
            group: string;
            hostname: string;
            id: string;
            identityFile?: string;
            jumpHostId?: string;
            lastUsedAt?: string;
            name: string;
            port: number;
            production?: boolean;
            proxyCommand?: string;
            updatedAt: string;
            username: string;
        };
        ImportApply: {
            aliases: string[];
            content?: string;
            group?: string;
            source: "local" | "text";
        };
        ImportCandidate: {
            alias: string;
            diagnostics: components["schemas"]["ImportDiagnostic"][];
            existing: boolean;
            hostname: string;
            identitiesOnly: boolean;
            identityFiles?: string[];
            importable: boolean;
            jump?: components["schemas"]["ImportJump"];
            port: number;
            proxyCommand?: string;
            useAgent: boolean;
            user: string;
        };
        ImportDiagnostic: {
            directive: string;
            level: "info" | "warning" | "blocked";
            line?: number;
            message: string;
            source?: string;
        };
        ImportJump: {
            alias?: string;
            hostname: string;
            port: number;
            user?: string;
        };
        ImportPreview: {
            candidates: components["schemas"]["ImportCandidate"][];
            diagnostics: components["schemas"]["ImportDiagnostic"][];
        };
        ImportResult: {
            created: components["schemas"]["HostProfile"][];
            skipped: {
                alias: string;
                reason: string;
            }[];
        };
        ImportSource: {
            content?: string;
            source: "local" | "text";
        };
        LaunchResult: {
            terminal?: components["schemas"]["TerminalInfo"];
            tunnel?: components["schemas"]["Tunnel"];
            warning?: string;
        };
        LocalDocument: {
            bom: boolean;
            content: string;
            id: string;
            lineEnding: "lf" | "crlf" | "mixed";
            path: string;
            version: string;
        };
        LoginRequest: {
            password: string;
        };
        Mutation: {
            directory?: boolean;
            expected?: number;
            mode?: string;
            name?: string;
            operation: "mkdir" | "rename" | "delete" | "chmod";
            path: string;
            recursive?: boolean;
        };
        Prompt: {
            address?: string;
            algorithm?: string;
            fingerprint?: string;
            hop: "target" | "jump";
            host: string;
            id: string;
            instruction?: string;
            kind: "host-key" | "keyboard-interactive";
            knownHosts?: "match" | "mismatch" | "absent";
            name?: string;
            questions?: components["schemas"]["Question"][];
        };
        Question: {
            echo: boolean;
            text: string;
        };
        QuickConnectTarget: {
            authMode?: "auto" | "password" | "key" | "agent" | "keyboard-interactive";
            hostname: string;
            identityFile?: string;
            name?: string;
            port: number;
            proxyCommand?: string;
            username?: string;
        };
        Snippet: {
            command: string;
            createdAt: string;
            description: string;
            id: string;
            name: string;
            updatedAt: string;
        };
        SnippetInput: {
            command: string;
            description: string;
            name: string;
        };
        SSHConfigHost: {
            alias: string;
            diagnostics: components["schemas"]["ImportDiagnostic"][];
            hostname: string;
            id: string;
            identitiesOnly: boolean;
            identityFiles?: string[];
            importable: boolean;
            jump?: components["schemas"]["ImportJump"];
            port: number;
            proxyCommand?: string;
            useAgent: boolean;
            user: string;
        };
        SSHConfigHosts: {
            diagnostics: components["schemas"]["ImportDiagnostic"][];
            hosts: components["schemas"]["SSHConfigHost"][];
        };
        TerminalInfo: {
            createdAt: string;
            exitCode?: number;
            exitSignal?: string;
            hostId: string;
            id: string;
            name: string;
            status: "running" | "closed" | "exited" | "disconnected";
        };
        TreeUsage: {
            bytes: number;
            directories: number;
            files: number;
            path: string;
            symlinks: number;
            unsupported: number;
        };
        Tunnel: {
            createdAt: string;
            hostId: string;
            id: string;
            listenHost: "127.0.0.1" | "::1";
            listenPort: number;
            name: string;
            status: components["schemas"]["TunnelStatus"];
            targetHost: string;
            targetPort: number;
            updatedAt: string;
        };
        TunnelInput: {
            hostId: string;
            listenHost: "127.0.0.1" | "::1";
            listenPort: number;
            name: string;
            targetHost: string;
            targetPort: number;
        };
        TunnelStatus: {
            activeConnections: number;
            bytesReceived: number;
            bytesSent: number;
            error?: components["schemas"]["Error"];
            listen?: string;
            startedAt?: string;
            state: "stopped" | "running" | "failed";
            totalConnections: number;
        };
        VaultReset: {
            confirm: "DELETE ALL SAVED CREDENTIALS";
        };
        VaultSetup: {
            mode: "keyring" | "password";
            password?: string;
        };
        VaultStatus: {
            initialized: boolean;
            mode?: "keyring" | "password";
            unlocked: boolean;
        };
        VaultUnlock: {
            password?: string;
        };
        Workspace: {
            layout: {
                sizes: number[];
                split: "single" | "columns" | "rows" | "grid";
                tabs: {
                    hostId: string;
                }[];
            };
            preferences: {
                cursorStyle: "bar" | "block" | "underline";
                fontSize: number;
                keybindings: {
                    [key: string]: string;
                };
                scrollback: number;
            };
            updatedAt?: string;
        };
    };
    responses: {
        Failure: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                "application/json": components["schemas"]["ErrorResponse"];
            };
        };
    };
    parameters: {
        CSRF: string;
    };
    requestBodies: never;
    headers: never;
    pathItems: never;
}
export type $defs = Record<string, never>;
export interface operations {
    startAttempt: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AttemptRequest"];
            };
        };
        responses: {
            202: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Attempt"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    waitAttempt: {
        parameters: {
            query?: {
                after?: number;
            };
            header?: never;
            path: {
                attemptID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Attempt"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    cancelAttempt: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path: {
                attemptID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            default: components["responses"]["Failure"];
        };
    };
    answerAttempt: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path: {
                attemptID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AttemptAnswer"];
            };
        };
        responses: {
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            default: components["responses"]["Failure"];
        };
    };
    authState: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AuthState"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    bootstrap: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["BootstrapRequest"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AuthState"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    login: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["LoginRequest"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AuthState"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    logout: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            default: components["responses"]["Failure"];
        };
    };
    listCredentials: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CredentialRecord"][];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    createCredential: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CredentialInput"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CredentialRecord"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    deleteCredential: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path: {
                credentialID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            default: components["responses"]["Failure"];
        };
    };
    listHosts: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["HostProfile"][];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    createHost: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["HostInput"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["HostProfile"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    updateHost: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path: {
                hostID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["HostInput"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["HostProfile"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    deleteHost: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path: {
                hostID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            default: components["responses"]["Failure"];
        };
    };
    applySSHConfig: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ImportApply"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ImportResult"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    previewSSHConfig: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ImportSource"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ImportPreview"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    readLocalDocument: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                documentID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["LocalDocument"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    saveLocalDocument: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path: {
                documentID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["Document"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["LocalDocument"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    listSnippets: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Snippet"][];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    createSnippet: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["SnippetInput"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Snippet"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    updateSnippet: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path: {
                snippetID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["SnippetInput"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Snippet"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    deleteSnippet: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path: {
                snippetID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            default: components["responses"]["Failure"];
        };
    };
    listSSHConfigHosts: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["SSHConfigHosts"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    listTerminals: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TerminalInfo"][];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    closeTerminal: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            default: components["responses"]["Failure"];
        };
    };
    downloadArchive: {
        parameters: {
            query: {
                path: string;
            };
            header?: never;
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/zip": string;
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    readDocument: {
        parameters: {
            query: {
                path: string;
            };
            header?: never;
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Document"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    saveDocument: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["Document"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Document"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    downloadFile: {
        parameters: {
            query: {
                path: string;
            };
            header?: never;
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/octet-stream": string;
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    listFiles: {
        parameters: {
            query?: {
                path?: string;
            };
            header?: never;
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["FileListing"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    mutateFile: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["Mutation"];
            };
        };
        responses: {
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            default: components["responses"]["Failure"];
        };
    };
    uploadFile: {
        parameters: {
            query: {
                name: string;
                path: string;
            };
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/octet-stream": string;
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            default: components["responses"]["Failure"];
        };
    };
    treeUsage: {
        parameters: {
            query: {
                path: string;
            };
            header?: never;
            path: {
                terminalID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TreeUsage"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    listTunnels: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Tunnel"][];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    createTunnel: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TunnelInput"];
            };
        };
        responses: {
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Tunnel"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    updateTunnel: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path: {
                tunnelID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TunnelInput"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Tunnel"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    deleteTunnel: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path: {
                tunnelID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            default: components["responses"]["Failure"];
        };
    };
    stopTunnel: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path: {
                tunnelID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            default: components["responses"]["Failure"];
        };
    };
    vaultStatus: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["VaultStatus"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    lockVault: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["VaultStatus"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    resetVault: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["VaultReset"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["VaultStatus"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    setupVault: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["VaultSetup"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["VaultStatus"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    unlockVault: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["VaultUnlock"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["VaultStatus"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    getWorkspace: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Workspace"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
    putWorkspace: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CSRF"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["Workspace"];
            };
        };
        responses: {
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Workspace"];
                };
            };
            default: components["responses"]["Failure"];
        };
    };
}
