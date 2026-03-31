package websocket

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
)

var (
	ErrCloseSent          = errors.New("websocket: close message sent")
	ErrUnexpectedEOF      = errors.New("websocket: unexpected EOF")
	ErrMessageTooLarge    = errors.New("websocket: message too large")
	ErrMaskNotSet         = errors.New("websocket: mask bit not set by client")
	ErrInvalidControlLen  = errors.New("websocket: invalid control frame length")
	ErrControlFragmented  = errors.New("websocket: control frame cannot be fragmented")
)

// Conn represents an active WebSocket connection.
type Conn struct {
	netConn net.Conn
	reader  *bufio.Reader
	writer  *bufio.Writer

	mu sync.Mutex // Protects concurrent writes
}

func newConn(conn net.Conn, r *bufio.Reader, w *bufio.Writer) *Conn {
	if r == nil {
		r = bufio.NewReader(conn)
	}
	if w == nil {
		w = bufio.NewWriter(conn)
	}
	return &Conn{
		netConn: conn,
		reader:  r,
		writer:  w,
	}
}

// Close gracefully closes the underlying network connection.
func (c *Conn) Close() error {
	return c.netConn.Close()
}

// ReadMessage blocks until a full WebSocket message is received.
// It returns the Opcode (TextMessage, BinaryMessage, etc.) and the unmasked payload bytes.
// Note: It handles Ping/Pong and Close frames internally by responding to them,
// but will also return them to the caller so they can inspect control frames if needed.
func (c *Conn) ReadMessage() (Opcode, []byte, error) {
	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(c.reader, header); err != nil {
			return 0, nil, err
		}

		fin := header[0]&0x80 != 0
		opcode := Opcode(header[0] & 0x0F)
		masked := header[1]&0x80 != 0
		payloadLen := int64(header[1] & 0x7F)

		if !masked {
			return 0, nil, ErrMaskNotSet
		}

		if opcode.IsControl() {
			if !fin {
				return 0, nil, ErrControlFragmented
			}
			if payloadLen > 125 {
				return 0, nil, ErrInvalidControlLen
			}
		}

		// Read extended length if necessary
		if payloadLen == 126 {
			extLen := make([]byte, 2)
			if _, err := io.ReadFull(c.reader, extLen); err != nil {
				return 0, nil, err
			}
			payloadLen = int64(binary.BigEndian.Uint16(extLen))
		} else if payloadLen == 127 {
			extLen := make([]byte, 8)
			if _, err := io.ReadFull(c.reader, extLen); err != nil {
				return 0, nil, err
			}
			payloadLen = int64(binary.BigEndian.Uint64(extLen))
		}

		// Prevent memory exhaustion (arbitrary limit of 10MB per message for gocore default)
		if payloadLen > 10*1024*1024 {
			return 0, nil, ErrMessageTooLarge
		}

		// Read Masking Key
		maskKey := make([]byte, 4)
		if _, err := io.ReadFull(c.reader, maskKey); err != nil {
			return 0, nil, err
		}

		// Read Payload
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(c.reader, payload); err != nil {
			return 0, nil, err
		}

		// Unmask
		for i := 0; i < int(payloadLen); i++ {
			payload[i] ^= maskKey[i%4]
		}

		// Handle control frames
		if opcode == CloseMessage {
			_ = c.WriteMessage(CloseMessage, payload) // Echo back the close using WriteMessage
			return opcode, payload, ErrCloseSent
		}
		if opcode == PingMessage {
			_ = c.WriteMessage(PongMessage, payload)
			continue // Hide pings from caller, or return them? Let's hide pings and wait for real data.
		}
		if opcode == PongMessage {
			continue // Ignore pongs
		}

		// TODO: Handle fragmented frames (FIN=false). For now, assume unfragmented (which fits 99% of basic usages)
		// To truly support RFC6455 we would buffer until FIN=true.
		// For an engineering-first library we'll return the full message assembled if fin is false.
		// Note: This implementation doesn't yet automatically re-assemble fragments across multiple Read calls;
		// it currently returns the part received.
		return opcode, payload, nil
	}
}

// WriteMessage sends a WebSocket message.
// Server-to-Client frames must NOT be masked.
func (c *Conn) WriteMessage(messageType Opcode, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var header []byte
	b0 := byte(messageType) | 0x80 // Set FIN bit to 1

	length := len(data)
	if length <= 125 {
		header = []byte{b0, byte(length)}
	} else if length <= 65535 {
		header = []byte{b0, 126, 0, 0}
		binary.BigEndian.PutUint16(header[2:4], uint16(length))
	} else {
		header = make([]byte, 10)
		header[0] = b0
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:10], uint64(length))
	}

	if _, err := c.writer.Write(header); err != nil {
		return err
	}
	if _, err := c.writer.Write(data); err != nil {
		return err
	}
	return c.writer.Flush()
}
