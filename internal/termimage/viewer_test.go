package termimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestPNGChunkReconstructionAndTargetedCleanup(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	// Non-compressible deterministic pixels force multiple protocol chunks.
	var state uint32 = 1
	for i := range img.Pix {
		state = state*1664525 + 1013904223
		img.Pix[i] = byte(state >> 24)
	}
	var pngBytes, wire bytes.Buffer
	if err := png.Encode(&pngBytes, img); err != nil {
		t.Fatal(err)
	}
	if err := transmit(&wire, pngBytes.Bytes(), 429); err != nil {
		t.Fatal(err)
	}
	packets := strings.Split(strings.TrimSuffix(wire.String(), "\x1b\\"), "\x1b\\")
	if len(packets) < 2 {
		t.Fatal("fixture did not exercise chunking")
	}
	var payload strings.Builder
	for i, packet := range packets {
		header, data, found := strings.Cut(packet, ";")
		if !found || !strings.HasPrefix(header, "\x1b_G") {
			t.Fatalf("invalid packet %q", header)
		}
		if len(data) > 4096 || len(data)%4 != 0 {
			t.Fatalf("invalid base64 chunk size %d", len(data))
		}
		more := "m=1"
		if i == len(packets)-1 {
			more = "m=0"
		}
		if !strings.HasSuffix(header, more) {
			t.Fatalf("invalid continuation flag %q", header)
		}
		if i == 0 {
			for _, field := range []string{"f=100", "a=t", "t=d", "q=0", "i=429"} {
				if !strings.Contains(header, field) {
					t.Fatalf("missing %s", field)
				}
			}
		}
		payload.WriteString(data)
	}
	reconstructed, err := base64.StdEncoding.DecodeString(payload.String())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reconstructed, pngBytes.Bytes()) {
		t.Fatal("PNG changed during chunking")
	}
	if _, err = png.Decode(bytes.NewReader(reconstructed)); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	if err = deleteImage(&wire, 429); err != nil {
		t.Fatal(err)
	}
	if wire.String() != "\x1b_Ga=d,d=I,i=429,q=2;\x1b\\" {
		t.Fatalf("unsafe deletion: %q", wire.String())
	}
}

func TestMultiplexerAlwaysUsesFallback(t *testing.T) {
	for _, mux := range []string{"TMUX", "STY"} {
		env := map[string]string{"TERM": "xterm-kitty", "TERM_PROGRAM": "ghostty", "KITTY_WINDOW_ID": "1", mux: "active"}
		if supportsGraphics(func(key string) string { return env[key] }) {
			t.Fatalf("native graphics enabled inside %s", mux)
		}
	}
	for _, name := range []string{"xterm-kitty", "xterm-ghostty"} {
		if !supportsGraphics(func(key string) string {
			if key == "TERM" {
				return name
			}
			return ""
		}) {
			t.Fatalf("not recognized: %s", name)
		}
	}
}

type forbiddenReader struct{ t *testing.T }

func (r forbiddenReader) Read([]byte) (int, error) {
	r.t.Fatal("non-TTY preview attempted blocking input")
	return 0, nil
}

func TestNonTTYFallbackRendersPixelsWithoutReading(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 255})
	var output bytes.Buffer
	viewer := New(context.Background(), img, "cover\x1b]2;injected\a\nname")
	viewer.SetStdin(forbiddenReader{t})
	viewer.SetStdout(&output)
	if err := viewer.Run(); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "\x1b[38;2;255;0;0m") || !strings.Contains(text, "\x1b[48;2;0;0;255m") || !strings.Contains(text, "▀") {
		t.Fatal("fallback did not render image colors")
	}
	if strings.Contains(text, "\x1b]") || strings.Contains(text, "\x1b_G") {
		t.Fatal("unsafe caption or native protocol in fallback")
	}
	for _, line := range strings.Split(text, "\r\n") {
		if ansi.StringWidth(line) > 79 {
			t.Fatal("preview overflowed width")
		}
	}
}

func TestGraphicsAcknowledgmentBoundaries(t *testing.T) {
	var parser responseParser
	response := []byte("\x1b_Gi=428;OK\x1b\\\x1b_Gi=429;OK\x1b\\")
	for i, value := range response {
		done, err := parser.feed([]byte{value}, 429)
		if err != nil {
			t.Fatalf("fragment %d: %v", i, err)
		}
		if done != (i == len(response)-1) {
			t.Fatalf("acknowledged wrong image or incomplete packet at %d", i)
		}
	}
	if _, err := parser.feed([]byte("q"), 429); !errors.Is(err, errPreviewClosed) {
		t.Fatalf("quit not honored: %v", err)
	}
}

func TestGraphicsAcknowledgmentRejectsTerminalErrors(t *testing.T) {
	var parser responseParser
	if done, err := parser.feed([]byte("\x1b_Gi=429;EINVAL: invalid image\x1b\\"), 429); done || err == nil {
		t.Fatal("terminal failure treated as successful upload")
	}
	parser = responseParser{}
	if _, err := parser.feed([]byte("\x1b_G"+strings.Repeat("x", 2048)), 429); err == nil {
		t.Fatal("unbounded terminal response accepted")
	}
}
