package requester

import (
	"bytes"

	"github.com/gorilla/websocket"
)

type wsReader[T streamable] struct {
	reader        *websocket.Conn
	handlerPrefix HandlerPrefix[T]

	DataChan chan T
	ErrChan  chan error
}

func (stream *wsReader[T]) Recv() (<-chan T, <-chan error) {
	go stream.processLines()
	return stream.DataChan, stream.ErrChan
}

func (stream *wsReader[T]) processLines() {
	// ✅ 确保函数退出时关闭 channels，防止 goroutine 泄漏
	defer close(stream.DataChan)
	defer close(stream.ErrChan)

	for {
		_, msg, err := stream.reader.ReadMessage()
		if err != nil {
			stream.ErrChan <- err
			return
		}

		stream.handlerPrefix(&msg, stream.DataChan, stream.ErrChan)

		if msg == nil {
			continue
		}

		if bytes.Equal(msg, StreamClosed) {
			return
		}
	}
}

func (stream *wsReader[T]) Close() {
	stream.reader.Close()
}
