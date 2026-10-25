// SPDX-License-Identifier: GPL-3.0-only

// Package edgetts 实现 Microsoft Edge 朗读服务的 WebSocket 合成协议，只请求 MP3。
// 协议参考 https://github.com/rany2/edge-tts 。
package edgetts

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	trustedClientToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	// 与上游保持一致：Bing 朗读端点使用 TrustedClientToken 和 GEC v2。
	endpoint        = "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1"
	chromiumVersion = "143.0.3650.75"
	chromiumMajor   = "143"
	outputFormat    = "audio-24khz-48kbitrate-mono-mp3"
	// 服务端在合成期间持续推送音频；超过该间隔没有任何消息即视为连接已失效。
	receiveTimeout = 20 * time.Second
	// 服务端单条消息远小于该值，超限说明协议异常。
	maxMessageBytes = 1 << 20
	// Windows 文件时间纪元（1601-01-01）与 Unix 纪元相差的秒数。
	windowsEpochOffset = 11644473600
)

var (
	// ErrEmptyAudio 表示服务端正常结束但没有返回音频，换一次请求通常可以恢复。
	ErrEmptyAudio = errors.New("edgetts: empty audio")
	// ErrAudioTooLarge 表示音频超过调用方给出的上限。
	ErrAudioTooLarge = errors.New("edgetts: audio exceeds limit")
	// ErrProtocol 表示服务端消息不符合协议。
	ErrProtocol = errors.New("edgetts: protocol error")
)

// HandshakeError 携带握手失败时的 HTTP 状态码，便于调用方区分拒绝与网络故障。
type HandshakeError struct{ Status int }

func (e *HandshakeError) Error() string {
	return fmt.Sprintf("edgetts: handshake rejected (HTTP %d)", e.Status)
}

// Error 标明失败所在的协议阶段，供调用方记录诊断；Unwrap 保留原始错误判定。
type Error struct {
	Stage string
	Err   error
}

func (e *Error) Error() string { return "edgetts: " + e.Stage + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// clockSkew 记录本机与服务端的时间差（纳秒）。令牌按服务端时间计算，
// 本机时钟偏差过大时服务端以 403 拒绝，下次请求使用校正后的时间。
var clockSkew atomic.Int64

// Synthesize 合成一段纯文本并返回 MP3。dialer 决定代理与握手超时；
// ctx 取消会立即关闭连接。limit 是允许的最大音频字节数。
func Synthesize(ctx context.Context, dialer *websocket.Dialer, voice, text string, limit int) (_ []byte, resultErr error) {
	stage := "encode_ssml"
	defer func() {
		if resultErr != nil {
			resultErr = &Error{Stage: stage, Err: resultErr}
		}
	}()
	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(text)); err != nil {
		return nil, err
	}
	stage = "handshake"
	conn, response, err := dialer.DialContext(ctx, requestURL(), requestHeader())
	if err != nil {
		if response != nil {
			adjustClock(response)
			return nil, &HandshakeError{Status: response.StatusCode}
		}
		return nil, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	conn.SetReadLimit(maxMessageBytes)
	timestamp := jsTimestamp()
	config := "X-Timestamp:" + timestamp + "\r\n" +
		"Content-Type:application/json; charset=utf-8\r\n" +
		"Path:speech.config\r\n\r\n" +
		`{"context":{"synthesis":{"audio":{"metadataoptions":{"sentenceBoundaryEnabled":"false","wordBoundaryEnabled":"true"},"outputFormat":"` + outputFormat + `"}}}}` + "\r\n"
	// 与官方客户端一致，SSML 消息的时间戳末尾带 Z。
	ssml := "X-RequestId:" + randomID() + "\r\n" +
		"Content-Type:application/ssml+xml\r\n" +
		"X-Timestamp:" + timestamp + "Z\r\n" +
		"Path:ssml\r\n\r\n" +
		"<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='en-US'>" +
		"<voice name='" + voice + "'><prosody pitch='+0Hz' rate='+0%' volume='+0%'>" + escaped.String() + "</prosody></voice></speak>"
	stage = "write_request"
	for _, message := range []string{config, ssml} {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
			return nil, contextError(ctx, err)
		}
	}
	var audio []byte
	for {
		stage = "read_message"
		if err := conn.SetReadDeadline(time.Now().Add(receiveTimeout)); err != nil {
			return nil, contextError(ctx, err)
		}
		kind, data, err := conn.ReadMessage()
		if err != nil {
			return nil, contextError(ctx, err)
		}
		stage = "decode_message"
		switch kind {
		case websocket.TextMessage:
			path, ok := messagePath(data)
			if !ok {
				return nil, ErrProtocol
			}
			if path == "turn.end" {
				if len(audio) == 0 {
					return nil, ErrEmptyAudio
				}
				return audio, nil
			}
		case websocket.BinaryMessage:
			if len(data) < 2 {
				return nil, ErrProtocol
			}
			size := int(binary.BigEndian.Uint16(data[:2]))
			if len(data) < 2+size {
				return nil, ErrProtocol
			}
			path, ok := messagePath(data[2 : 2+size])
			if !ok || path != "audio" {
				return nil, ErrProtocol
			}
			payload := data[2+size:]
			if len(audio)+len(payload) > limit {
				return nil, ErrAudioTooLarge
			}
			audio = append(audio, payload...)
		}
	}
}

// contextError 优先返回取消原因：ctx 结束时关闭连接产生的读写错误没有诊断价值。
func contextError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return err
}

// messagePath 读取消息头部中的 Path。文本消息的头部以空行结束，二进制消息的头部单独成段。
func messagePath(data []byte) (string, bool) {
	head, _, _ := bytes.Cut(data, []byte("\r\n\r\n"))
	path := ""
	for line := range strings.SplitSeq(string(head), "\r\n") {
		if line == "" {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			return "", false
		}
		if strings.EqualFold(strings.TrimSpace(key), "Path") {
			path = strings.TrimSpace(value)
		}
	}
	return path, path != ""
}

func requestURL() string {
	return endpoint + "?TrustedClientToken=" + trustedClientToken +
		"&Sec-MS-GEC=" + secMSGEC(time.Now()) +
		"&Sec-MS-GEC-Version=2-" + chromiumVersion +
		"&ConnectionId=" + randomID()
}

func requestHeader() http.Header {
	header := http.Header{}
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/"+chromiumMajor+".0.0.0 Safari/537.36 Edg/"+chromiumMajor+".0.0.0")
	header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	header.Set("Accept-Language", "en-US,en;q=0.9")
	header.Set("Pragma", "no-cache")
	header.Set("Cache-Control", "no-cache")
	header.Set("Origin", "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold")
	header.Set("Cookie", "muid="+strings.ToUpper(randomID())+";")
	return header
}

// secMSGEC 按服务端时间的五分钟窗口生成 v2 的 CS64 与定制 SHA-256 摘要。
func secMSGEC(now time.Time) string {
	seconds := now.Add(time.Duration(clockSkew.Load())).Unix() + windowsEpochOffset
	seconds -= seconds % 300
	var salt [64]byte
	data := fmt.Appendf(salt[:0], "%d%s", seconds*10_000_000, trustedClientToken)
	digest := gecSHA256(data)
	cs64 := gecCS64(data, digest)
	return strings.ToUpper(hex.EncodeToString(cs64[:]) + "-" + hex.EncodeToString(digest[:]))
}

// adjustClock 依据拒绝响应的 Date 头校正本机时钟偏差。
func adjustClock(response *http.Response) {
	if response.StatusCode != http.StatusForbidden {
		return
	}
	server, err := http.ParseTime(response.Header.Get("Date"))
	if err != nil {
		return
	}
	clockSkew.Store(int64(time.Until(server)))
}

// jsTimestamp 返回服务端要求的 JavaScript Date 字符串格式。
func jsTimestamp() string {
	return time.Now().UTC().Format("Mon Jan 02 2006 15:04:05 GMT+0000 (Coordinated Universal Time)")
}

func randomID() string {
	var id [16]byte
	_, _ = rand.Read(id[:])
	return hex.EncodeToString(id[:])
}
