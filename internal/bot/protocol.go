package bot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
	"unicode/utf8"
)

const (
	maxFrameBytes = 64 * 1024
	maxBatchSize  = 32
	callTimeout   = 30 * time.Second
)

// Error 仅携带可向客户端公开的错误，不包含底层诊断信息。
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Handler 必须遵守上下文取消；串行调用不另启可泄漏的请求协程。
type Handler interface {
	Call(context.Context, string, json.RawMessage) (any, *Error)
}

// Serve 处理逐行 JSON-RPC；取消时关闭流以解除阻塞，正常 EOF 不关闭调用方的流。
func Serve(ctx context.Context, input io.ReadCloser, output io.WriteCloser, handler Handler) error {
	stopped := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			// 标准流可能共享打开状态，逆序关闭以正确恢复描述符标志。
			_ = output.Close()
			_ = input.Close()
		case <-stopped:
		}
	}()
	defer func() {
		close(stopped)
		<-watcherDone
	}()

	// 多留两个字节容纳 CRLF；ReadSlice 不会为了超长帧扩容。
	reader := bufio.NewReaderSize(input, maxFrameBytes+2)
	for {
		if ctx.Err() != nil {
			return nil
		}
		frame, readErr := reader.ReadSlice('\n')
		if ctx.Err() != nil {
			return nil
		}
		if len(frame) > 0 && frame[len(frame)-1] == '\n' {
			frame = frame[:len(frame)-1]
			if len(frame) > 0 && frame[len(frame)-1] == '\r' {
				frame = frame[:len(frame)-1]
			}
		}
		if errors.Is(readErr, bufio.ErrBufferFull) || len(frame) > maxFrameBytes {
			return errors.New("bot input frame exceeds 65536 bytes")
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return errors.New("bot input read failed")
		}
		if len(frame) == 0 && errors.Is(readErr, io.EOF) {
			return nil
		}
		response := dispatchFrame(ctx, frame, handler)
		if ctx.Err() != nil {
			return nil
		}
		if response != nil {
			response = append(response, '\n')
			for len(response) > 0 {
				n, err := output.Write(response)
				if ctx.Err() != nil {
					return nil
				}
				if err != nil || n <= 0 || n > len(response) {
					return errors.New("bot output write failed")
				}
				response = response[n:]
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
	}
}

func dispatchFrame(ctx context.Context, frame []byte, handler Handler) []byte {
	if !utf8.Valid(frame) || !json.Valid(frame) {
		return errorResponse(nil, &Error{Code: -32700, Message: "Parse error"})
	}
	frame = bytes.TrimSpace(frame)
	if frame[0] != '[' {
		return dispatchRequest(ctx, frame, handler)
	}
	var members []json.RawMessage
	_ = json.Unmarshal(frame, &members)
	// 整批先检查数量，避免被拒绝的批次已产生发送等副作用。
	if len(members) == 0 || len(members) > maxBatchSize {
		return errorResponse(nil, &Error{Code: -32600, Message: "Invalid Request"})
	}
	var response []byte
	for _, member := range members {
		if ctx.Err() != nil {
			return nil
		}
		part := dispatchRequest(ctx, member, handler)
		if part == nil {
			continue
		}
		if response == nil {
			response = append(response, '[')
		} else {
			response = append(response, ',')
		}
		response = append(response, part...)
	}
	if response != nil {
		response = append(response, ']')
	}
	return response
}

func dispatchRequest(ctx context.Context, request []byte, handler Handler) []byte {
	request = bytes.TrimSpace(request)
	if len(request) == 0 || request[0] != '{' {
		return errorResponse(nil, &Error{Code: -32600, Message: "Invalid Request"})
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(request, &fields)
	var version, method string
	versionRaw, methodRaw := fields["jsonrpc"], fields["method"]
	id, hasID := fields["id"]
	if len(versionRaw) == 0 || versionRaw[0] != '"' || json.Unmarshal(versionRaw, &version) != nil || version != "2.0" ||
		len(methodRaw) == 0 || methodRaw[0] != '"' || json.Unmarshal(methodRaw, &method) != nil ||
		(hasID && !validID(id)) {
		return errorResponse(nil, &Error{Code: -32600, Message: "Invalid Request"})
	}
	params := fields["params"]
	if len(params) > 0 && params[0] != '{' && params[0] != '[' {
		if !hasID {
			return nil
		}
		return errorResponse(id, &Error{Code: -32602, Message: "Invalid params"})
	}
	if ctx.Err() != nil {
		return nil
	}
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	result, rpcErr := invokeHandler(callCtx, handler, method, params)
	if callCtx.Err() != nil {
		result, rpcErr = nil, &Error{Code: -32000, Message: "Request canceled or timed out"}
	}
	cancel()
	if !hasID {
		return nil
	}
	if rpcErr != nil {
		return errorResponse(id, rpcErr)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return errorResponse(id, &Error{Code: -32603, Message: "Internal error"})
	}
	return makeResponse(id, "result", encoded)
}

func validID(id json.RawMessage) bool {
	if len(id) == 0 {
		return false
	}
	return id[0] == '"' || id[0] == '-' || (id[0] >= '0' && id[0] <= '9') || bytes.Equal(id, []byte("null"))
}

func invokeHandler(ctx context.Context, handler Handler, method string, params json.RawMessage) (result any, rpcErr *Error) {
	defer func() {
		if recover() != nil {
			result, rpcErr = nil, &Error{Code: -32603, Message: "Internal error"}
		}
	}()
	return handler.Call(ctx, method, params)
}

func errorResponse(id json.RawMessage, rpcErr *Error) []byte {
	encoded, _ := json.Marshal(rpcErr)
	return makeResponse(id, "error", encoded)
}

func makeResponse(id json.RawMessage, field string, value []byte) []byte {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	// 原样拼入已验证的 ID，保留大数、指数及字符串转义，不经浮点或二次编码。
	response := make([]byte, 0, len(id)+len(field)+len(value)+32)
	response = append(response, `{"jsonrpc":"2.0","`...)
	response = append(response, field...)
	response = append(response, `":`...)
	response = append(response, value...)
	response = append(response, `,"id":`...)
	response = append(response, id...)
	response = append(response, '}')
	return response
}
