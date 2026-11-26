package terminals

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestStreamPreservesEveryUTF8Split(test *testing.T) {
	content := []byte("中文😀e\u0301\x1b[31m颜色\x1b[0m\r\n")
	for split := 1; split < len(content); split++ {
		stream := NewStream(1024)
		ctx := context.Background()
		if err := stream.Append(ctx, content[:split]); err != nil {
			test.Fatal(err)
		}
		if err := stream.Append(ctx, content[split:]); err != nil {
			test.Fatal(err)
		}
		stream.Close()
		var output []byte
		var offset uint64
		for {
			chunk, err := stream.Next(ctx, offset)
			if err == io.EOF {
				break
			}
			if err != nil {
				test.Fatal(err)
			}
			output = append(output, chunk.Data...)
			offset += uint64(len(chunk.Data))
			if err := stream.Ack(offset); err != nil {
				test.Fatal(err)
			}
		}
		if !bytes.Equal(output, content) {
			test.Fatalf("split %d changed bytes: %q", split, output)
		}
	}
}

func TestStreamBackpressureAndCancellation(test *testing.T) {
	stream := NewStream(8)
	if err := stream.Append(context.Background(), []byte("12345678")); err != nil {
		test.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := stream.Append(ctx, []byte("9")); !errors.Is(err, context.DeadlineExceeded) {
		test.Fatalf("expected bounded backpressure, got %v", err)
	}
	if err := stream.Ack(4); err != nil {
		test.Fatal(err)
	}
	if err := stream.Append(context.Background(), []byte("90")); err != nil {
		test.Fatal(err)
	}
	chunk, err := stream.Next(context.Background(), 4)
	if err != nil || string(chunk.Data) != "5678" {
		test.Fatalf("resume mismatch: %q %v", chunk.Data, err)
	}
	if err := stream.Ack(11); !errors.Is(err, ErrOffsetUnavailable) {
		test.Fatalf("accepted future ACK: %v", err)
	}
	if _, err := stream.Next(context.Background(), 3); !errors.Is(err, ErrOffsetUnavailable) {
		test.Fatalf("accepted discarded history: %v", err)
	}
}

func TestClosingStreamUnblocksProducer(test *testing.T) {
	stream := NewStream(1)
	if err := stream.Append(context.Background(), []byte("x")); err != nil {
		test.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- stream.Append(context.Background(), []byte("y")) }()
	stream.Close()
	select {
	case err := <-finished:
		if !errors.Is(err, io.ErrClosedPipe) {
			test.Fatal(err)
		}
	case <-time.After(time.Second):
		test.Fatal("producer leaked")
	}
}
