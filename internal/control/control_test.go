package control

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	edited   string
	stopOnce sync.Once
	stopped  chan struct{}
}

func (handler *recorder) Open() (string, error) { return "http://127.0.0.1:1/#bootstrap=x", nil }
func (handler *recorder) Edit(path string) (string, error) {
	handler.edited = path
	if path == "" {
		return "", errors.New("absolute path required")
	}
	return "http://127.0.0.1:1/local-editor?document=d", nil
}
func (handler *recorder) Status() Response {
	return Response{Origin: "http://127.0.0.1:1", Listen: "127.0.0.1:1", Version: "test", PID: 42}
}
func (handler *recorder) Stop() {
	handler.stopOnce.Do(func() {
		if handler.stopped != nil {
			close(handler.stopped)
		}
	})
}

func TestControlRequestsRoundTrip(test *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		test.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handler := &recorder{stopped: make(chan struct{})}
	go Serve(ctx, listener, handler)
	dial := func() net.Conn {
		connection, err := net.Dial("tcp", listener.Addr().String())
		if err != nil {
			test.Fatal(err)
		}
		return connection
	}
	if response, err := Call(dial(), Request{Command: "open"}); err != nil || response.URL == "" || response.PID != 42 {
		test.Fatalf("open: %+v %v", response, err)
	}
	if response, err := Call(dial(), Request{Command: "status"}); err != nil || response.Origin != "http://127.0.0.1:1" || response.PID != 42 {
		test.Fatalf("status: %+v %v", response, err)
	}
	if _, err := Call(dial(), Request{Command: "edit", Path: "/tmp/a.txt"}); err != nil || handler.edited != "/tmp/a.txt" {
		test.Fatalf("edit: %v", err)
	}
	if _, err := Call(dial(), Request{Command: "edit"}); err == nil {
		test.Fatal("handler error not returned")
	}
	if _, err := Call(dial(), Request{Command: "shell"}); err == nil {
		test.Fatal("unknown command accepted")
	}
	if response, err := Call(dial(), Request{Command: "stop"}); err != nil || !response.Stopped {
		test.Fatalf("stop: %+v err=%v", response, err)
	}
	select {
	case <-handler.stopped:
	case <-time.After(time.Second):
		test.Fatal("stop handler was not invoked")
	}
}
