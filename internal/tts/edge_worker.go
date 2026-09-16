package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	edge "github.com/wujunwei928/edge-tts-go/edge_tts"
)

// RunEdge handles one request in a dedicated process. It changes that process's
// proxy environment and default logger; callers must not run it in the host.
func RunEdge(in io.Reader, out io.Writer) error {
	request, err := readEdgeRequest(in)
	if err != nil {
		return err
	}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if proxyEnv(key) {
			if os.Unsetenv(key) != nil {
				return errors.New("tts: proxy environment cleanup failed")
			}
		}
	}
	log.SetOutput(io.Discard)
	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(request.Text)); err != nil {
		return errors.New("tts: invalid synthesis input")
	}
	communicator, err := edge.NewCommunicate(escaped.String(), edge.SetVoice(request.Voice), edge.SetProxy(request.Proxy), edge.SetOutputFormat(edge.OutputFormatMP3), edge.SetReceiveTimeout(20))
	if err != nil {
		return errors.New("tts: invalid synthesis configuration")
	}
	audio, err := communicator.Stream()
	if err != nil {
		return safeEdgeError(err)
	}
	if len(audio) == 0 {
		return errors.New("tts: synthesis returned empty audio")
	}
	if len(audio) > MaxAudioBytes {
		return errors.New("tts: synthesis audio exceeds limit")
	}
	if _, err := out.Write(audio); err != nil {
		return errors.New("tts: writing synthesis audio failed")
	}
	return nil
}
func readEdgeRequest(in io.Reader) (edgeRequest, error) {
	var request edgeRequest
	data, err := io.ReadAll(io.LimitReader(in, (8<<10)+1))
	if err != nil || len(data) > 8<<10 || !utf8.Valid(data) {
		return request, errors.New("tts: invalid synthesis input")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, errors.New("tts: invalid synthesis input")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return request, errors.New("tts: invalid synthesis input")
	}
	request.Text, err = validateText(request.Text)
	if err != nil {
		return request, err
	}
	if !validVoice(request.Voice) {
		return request, errors.New("tts: invalid voice")
	}
	request.Proxy, err = normalizeProxy(request.Proxy)
	if err != nil {
		return request, err
	}
	return request, nil
}
func safeEdgeError(err error) error {
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
		return errors.New("tts: synthesis network timeout")
	}
	if errors.Is(err, websocket.ErrBadHandshake) {
		return errors.New("tts: synthesis handshake rejected")
	}
	return errors.New("tts: synthesis network or protocol failure")
}
