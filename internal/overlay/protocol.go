package overlay

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

// 每帧为四字节大端长度加 UTF-8 JSON；不依赖控制台编码，
// 文本中的换行不会被误当作消息边界。接收前先限制长度，再解析内容。
const protocolVersion = 1
const maxFrameBytes = 6*(MaxTextBytes+64*1024) + 16*1024
const tokenEnvironment = "ARCANA_OVERLAY_TOKEN"
const writeTimeout = 2 * time.Second

type wireFrame struct {
	Version int     `json:"version"`
	Type    string  `json:"type"`
	Token   string  `json:"token,omitempty"`
	Config  *Config `json:"config,omitempty"`
	Error   string  `json:"error,omitempty"`
}

func readFrame(r io.Reader) (wireFrame, error) {
	var frame wireFrame
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return frame, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > maxFrameBytes {
		return frame, fmt.Errorf("overlay: invalid frame size %d", size)
	}
	data := make([]byte, int(size))
	if _, err := io.ReadFull(r, data); err != nil {
		return frame, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&frame); err != nil {
		return frame, fmt.Errorf("overlay: invalid frame: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return frame, fmt.Errorf("overlay: trailing frame data")
	}
	if frame.Version != protocolVersion {
		return frame, fmt.Errorf("overlay: protocol version %d, want %d", frame.Version, protocolVersion)
	}
	return frame, nil
}

func writeFrame(conn net.Conn, frame wireFrame) error {
	frame.Version = protocolVersion
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	if len(data) > maxFrameBytes {
		return fmt.Errorf("overlay: frame exceeds %d bytes", maxFrameBytes)
	}
	if err = conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	buffers := net.Buffers{header[:], data}
	_, err = buffers.WriteTo(conn)
	return err
}
