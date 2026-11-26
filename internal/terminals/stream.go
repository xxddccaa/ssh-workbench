package terminals

import (
	"context"
	"errors"
	"io"
	"sync"
)

var ErrOffsetUnavailable = errors.New("output offset is no longer available")

const FrameLimit = 32 * 1024
const BufferLimit = 8 * 1024 * 1024
const WindowLimit = 256 * 1024

type Chunk struct {
	Offset uint64
	Data   []byte
}

type Stream struct {
	mu      sync.Mutex
	chunks  []Chunk
	base    uint64
	next    uint64
	limit   int
	closed  bool
	changed chan struct{}
}

func NewStream(limit int) *Stream {
	return &Stream{limit: limit, changed: make(chan struct{})}
}

func (stream *Stream) signalLocked() {
	close(stream.changed)
	stream.changed = make(chan struct{})
}

func (stream *Stream) Append(ctx context.Context, data []byte) error {
	for len(data) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		length := min(len(data), FrameLimit, stream.limit)
		stream.mu.Lock()
		if stream.closed {
			stream.mu.Unlock()
			return io.ErrClosedPipe
		}
		if stream.next-stream.base+uint64(length) <= uint64(stream.limit) {
			stream.chunks = append(stream.chunks, Chunk{Offset: stream.next, Data: append([]byte(nil), data[:length]...)})
			stream.next += uint64(length)
			stream.signalLocked()
			stream.mu.Unlock()
			data = data[length:]
			continue
		}
		changed := stream.changed
		stream.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
	return nil
}

func (stream *Stream) Next(ctx context.Context, offset uint64) (Chunk, error) {
	for {
		stream.mu.Lock()
		if offset < stream.base || offset > stream.next {
			stream.mu.Unlock()
			return Chunk{}, ErrOffsetUnavailable
		}
		for _, chunk := range stream.chunks {
			end := chunk.Offset + uint64(len(chunk.Data))
			if offset >= chunk.Offset && offset < end {
				result := Chunk{Offset: offset, Data: chunk.Data[offset-chunk.Offset:]}
				stream.mu.Unlock()
				return result, nil
			}
		}
		if stream.closed {
			stream.mu.Unlock()
			return Chunk{}, io.EOF
		}
		changed := stream.changed
		stream.mu.Unlock()
		select {
		case <-ctx.Done():
			return Chunk{}, ctx.Err()
		case <-changed:
		}
	}
}

func (stream *Stream) Ack(offset uint64) error {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if offset < stream.base || offset > stream.next {
		return ErrOffsetUnavailable
	}
	for len(stream.chunks) > 0 {
		chunk := stream.chunks[0]
		end := chunk.Offset + uint64(len(chunk.Data))
		if end > offset {
			if offset > chunk.Offset {
				stream.chunks[0] = Chunk{Offset: offset, Data: chunk.Data[offset-chunk.Offset:]}
			}
			break
		}
		stream.chunks[0] = Chunk{}
		stream.chunks = stream.chunks[1:]
	}
	stream.base = offset
	stream.signalLocked()
	return nil
}

func (stream *Stream) Close() {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if !stream.closed {
		stream.closed = true
		stream.signalLocked()
	}
}
