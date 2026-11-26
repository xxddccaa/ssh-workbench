// Package control is the local, OS-user-only command channel between the
// CLI and a running server. It carries no SSH credentials; it only asks the
// server for a fresh one-time browser authorization or registers a local
// file the user named on the command line.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"time"
)

const requestLimit = 64 * 1024

type Request struct {
	Version int    `json:"version"`
	Command string `json:"command"`
	Path    string `json:"path,omitempty"`
}

type Response struct {
	URL     string `json:"url,omitempty"`
	Origin  string `json:"origin,omitempty"`
	Listen  string `json:"listen,omitempty"`
	Version string `json:"version,omitempty"`
	PID     int    `json:"pid,omitempty"`
	Remote  bool   `json:"remote,omitempty"`
	Stopped bool   `json:"stopped,omitempty"`
	Error   string `json:"error,omitempty"`
}

type Handler interface {
	Open() (string, error)
	Edit(string) (string, error)
	Status() Response
	Stop()
}

func Serve(ctx context.Context, listener net.Listener, handler Handler) {
	stop := context.AfterFunc(ctx, func() { listener.Close() })
	defer stop()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
				slog.Warn("control channel stopped")
			}
			return
		}
		go respond(connection, handler)
	}
}

func respond(connection net.Conn, handler Handler) {
	defer connection.Close()
	connection.SetDeadline(time.Now().Add(10 * time.Second))
	var request Request
	if err := json.NewDecoder(io.LimitReader(connection, requestLimit)).Decode(&request); err != nil {
		json.NewEncoder(connection).Encode(Response{Error: "invalid control request"})
		return
	}
	var response Response
	var err error
	var afterReply func()
	switch {
	case request.Version != 1:
		err = errors.New("incompatible control protocol; restart the server with the same version")
	case request.Command == "open":
		status := handler.Status()
		var openURL string
		openURL, err = handler.Open()
		if err == nil {
			response = status
			response.URL = openURL
		}
	case request.Command == "edit":
		status := handler.Status()
		var editURL string
		editURL, err = handler.Edit(request.Path)
		if err == nil {
			response = status
			response.URL = editURL
		}
	case request.Command == "status":
		response = handler.Status()
	case request.Command == "stop":
		status := handler.Status()
		response = Response{Stopped: true, PID: status.PID, Origin: status.Origin, Listen: status.Listen}
		afterReply = handler.Stop
	default:
		err = errors.New("unknown control command")
	}
	if err != nil {
		response = Response{Error: err.Error()}
	}
	json.NewEncoder(connection).Encode(response)
	if afterReply != nil {
		afterReply()
	}
}

// Call sends one request over an already dialed control connection.
func Call(connection net.Conn, request Request) (Response, error) {
	defer connection.Close()
	connection.SetDeadline(time.Now().Add(10 * time.Second))
	request.Version = 1
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return Response{}, err
	}
	var response Response
	if err := json.NewDecoder(io.LimitReader(connection, requestLimit)).Decode(&response); err != nil {
		return Response{}, err
	}
	if response.Error != "" {
		return response, errors.New(response.Error)
	}
	return response, nil
}
