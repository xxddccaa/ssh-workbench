package httpapi

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"sshworkbench/internal/fault"
	"sshworkbench/internal/terminals"
)

func (server *Server) terminalSocket(response http.ResponseWriter, request *http.Request) {
	session, err := server.Terminals.Get(owner(request), request.PathValue("terminalID"))
	if err != nil {
		writeError(response, err)
		return
	}
	offset, err := strconv.ParseUint(request.URL.Query().Get("offset"), 10, 64)
	if err != nil {
		writeError(response, fault.New(400, "invalid_offset", "终端偏移无效。"))
		return
	}
	if err := session.Attach(offset); err != nil {
		writeError(response, err)
		return
	}
	defer session.Detach()
	connection, err := websocket.Accept(response, request, &websocket.AcceptOptions{OriginPatterns: server.originPatterns(), CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer connection.CloseNow()
	connection.SetReadLimit(terminals.FrameLimit + 9)
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	var acknowledged atomic.Uint64
	var sent atomic.Uint64
	acknowledged.Store(offset)
	sent.Store(offset)
	credit := make(chan struct{}, 1)
	go func() {
		defer cancel()
		defer connection.Close(websocket.StatusPolicyViolation, "terminal input ended")
		for {
			messageType, data, err := connection.Read(ctx)
			if err != nil {
				return
			}
			if messageType == websocket.MessageBinary {
				if len(data) < 2 || data[0] != 2 || len(data)-1 > terminals.FrameLimit {
					return
				}
				if err := session.Write(data[1:]); err != nil {
					return
				}
				continue
			}
			var control struct {
				Type   string `json:"type"`
				Offset string `json:"offset,omitempty"`
				Rows   int    `json:"rows,omitempty"`
				Cols   int    `json:"cols,omitempty"`
			}
			if err := json.Unmarshal(data, &control); err != nil {
				return
			}
			switch control.Type {
			case "ack":
				confirmed, err := strconv.ParseUint(control.Offset, 10, 64)
				if err != nil || confirmed < acknowledged.Load() || confirmed > sent.Load() {
					return
				}
				if err := session.Stream().Ack(confirmed); err != nil {
					return
				}
				acknowledged.Store(confirmed)
				select {
				case credit <- struct{}{}:
				default:
				}
			case "resize":
				if err := session.Resize(control.Rows, control.Cols); err != nil {
					return
				}
			default:
				return
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pingContext, cancelPing := context.WithTimeout(ctx, 5*time.Second)
				err := connection.Ping(pingContext)
				cancelPing()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	hello, _ := json.Marshal(map[string]any{"type": "hello", "version": 1, "offset": strconv.FormatUint(offset, 10)})
	if err := connection.Write(ctx, websocket.MessageText, hello); err != nil {
		return
	}
	for {
		outstanding := offset - acknowledged.Load()
		if outstanding >= terminals.WindowLimit {
			select {
			case <-ctx.Done():
				return
			case <-credit:
				continue
			}
		}
		chunk, err := session.Stream().Next(ctx, offset)
		if err != nil {
			if err == io.EOF {
				info := session.Info()
				message := map[string]any{"type": "state", "status": info.Status}
				if info.ExitCode != nil {
					message["exitCode"] = *info.ExitCode
				}
				if info.ExitSignal != "" {
					message["exitSignal"] = info.ExitSignal
				}
				state, _ := json.Marshal(message)
				connection.Write(ctx, websocket.MessageText, state)
				connection.Close(websocket.StatusNormalClosure, "terminal ended")
			}
			return
		}
		available := terminals.WindowLimit - outstanding
		data := chunk.Data[:min(uint64(len(chunk.Data)), available)]
		frame := make([]byte, 9+len(data))
		frame[0] = 1
		binary.BigEndian.PutUint64(frame[1:9], chunk.Offset)
		copy(frame[9:], data)
		sent.Store(offset + uint64(len(data)))
		writeContext, cancelWrite := context.WithTimeout(ctx, 10*time.Second)
		err = connection.Write(writeContext, websocket.MessageBinary, frame)
		cancelWrite()
		if err != nil {
			return
		}
		offset += uint64(len(data))
	}
}

func (server *Server) originPatterns() []string {
	if !server.config.RequireAuth {
		return []string{"*"}
	}
	patterns := []string{server.origin.Host}
	if server.config.DevOrigin != "" {
		patterns = append(patterns, server.config.DevOrigin[len("http://"):])
	}
	return patterns
}
