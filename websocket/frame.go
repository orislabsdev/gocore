package websocket

// Opcode represents a WebSocket frame opcode.
type Opcode byte

const (
	// ContinuationFrame represents a continuation frame.
	ContinuationFrame Opcode = 0x0
	// TextMessage represents a text data frame.
	TextMessage Opcode = 0x1
	// BinaryMessage represents a binary data frame.
	BinaryMessage Opcode = 0x2
	// CloseMessage represents a connection close control frame.
	CloseMessage Opcode = 0x8
	// PingMessage represents a ping control frame.
	PingMessage Opcode = 0x9
	// PongMessage represents a pong control frame.
	PongMessage Opcode = 0xA
)

// IsControl returns true if the opcode is a control frame (Close, Ping, Pong).
// Control frames have opcodes with the highest bit set to 1.
func (o Opcode) IsControl() bool {
	return (o & 0x8) != 0
}
