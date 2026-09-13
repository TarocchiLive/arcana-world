// Package termimage displays images while the application's terminal renderer is suspended.
package termimage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"os"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

// Viewer structurally implements tea.ExecCommand. Run is the sole output owner.
// Non-terminal output receives one ANSI half-block frame and returns immediately.
// Interactive input must be a terminal file; arbitrary blocking readers are never
// read, avoiding uncancellable reader goroutines.
type Viewer struct {
	ctx     context.Context
	img     image.Image
	caption string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func New(ctx context.Context, img image.Image, caption string) *Viewer {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Viewer{ctx: ctx, img: img, caption: caption, stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr}
}

func (v *Viewer) SetStdin(r io.Reader)  { v.stdin = r }
func (v *Viewer) SetStdout(w io.Writer) { v.stdout = w }
func (v *Viewer) SetStderr(w io.Writer) { v.stderr = w }

type dimensions struct{ cols, rows, width, height int }

func (v *Viewer) Run() (result error) {
	if v.img == nil || v.img.Bounds().Empty() {
		return errors.New("preview: empty image")
	}
	if v.stdout == nil {
		return errors.New("preview: no output")
	}
	if err := v.ctx.Err(); err != nil {
		return err
	}
	out, outputFile := v.stdout.(*os.File)
	if !outputFile || !term.IsTerminal(out.Fd()) {
		return v.frame(dimensions{cols: 80, rows: 24}, false, 0, nil)
	}
	in, ok := v.stdin.(*os.File)
	if !ok || !term.IsTerminal(in.Fd()) {
		return errors.New("preview: interactive input requires a terminal file")
	}
	if !interactiveSupported {
		return errors.New("preview: interactive terminal input is unsupported on this platform")
	}
	state, err := term.MakeRaw(in.Fd())
	if err != nil {
		return fmt.Errorf("preview: raw input: %w", err)
	}
	defer func() { result = errors.Join(result, term.Restore(in.Fd(), state)) }()
	var random [4]byte
	if _, err = rand.Read(random[:]); err != nil {
		return err
	}
	id := binary.BigEndian.Uint32(random[:])
	if id == 0 {
		id = 1
	}
	var encoded bytes.Buffer
	native := supportsGraphics(os.Getenv)
	if native {
		if err = png.Encode(&encoded, v.img); err != nil {
			return err
		}
	}
	// Cleanup is installed before the first write, including partial-write failures.
	defer func() {
		if native {
			result = errors.Join(result, deleteImage(v.stdout, id))
		}
		_, err := io.WriteString(v.stdout, "\x1b[0m\x1b[?25h\x1b[?1049l")
		result = errors.Join(result, err)
	}()
	if _, err = io.WriteString(v.stdout, "\x1b[?1049h\x1b[?25l"); err != nil {
		return err
	}
	var previous dimensions
	var input [64]byte
	for {
		if err = v.ctx.Err(); err != nil {
			return err
		}
		size, sizeErr := terminalSize(out)
		if sizeErr != nil {
			return fmt.Errorf("preview: terminal size: %w", sizeErr)
		}
		if size != previous {
			// Delete only our payload before resizing; never leave stale placements.
			if native {
				if err = deleteImage(v.stdout, id); err != nil {
					return err
				}
			}
			if _, err = io.WriteString(v.stdout, "\x1b[2J\x1b[H"); err != nil {
				return err
			}
			// Without pixel geometry an aspect-preserving native fit cannot be
			// bounded reliably. Half-block rendering stays within the cell grid.
			if err = v.frame(size, native && size.width > 0 && size.height > 0, id, encoded.Bytes()); err != nil {
				if errors.Is(err, errPreviewClosed) {
					return nil
				}
				if errors.Is(err, errPreviewResized) {
					continue
				}
				return err
			}
			previous = size
		}
		n, readErr := pollInput(in, input[:])
		if readErr != nil {
			return readErr
		}
		for _, key := range input[:n] {
			if key == 27 || key == 'q' || key == 'Q' || key == '\r' || key == '\n' || key == 3 {
				return nil
			}
		}
	}
}

func supportsGraphics(getenv func(string) string) bool {
	name := strings.ToLower(getenv("TERM"))
	if getenv("TMUX") != "" || getenv("STY") != "" || strings.HasPrefix(name, "screen") || strings.HasPrefix(name, "tmux") {
		return false
	}
	return name == "xterm-kitty" || name == "xterm-ghostty" || strings.EqualFold(getenv("TERM_PROGRAM"), "ghostty") || getenv("KITTY_WINDOW_ID") != ""
}

func cleanCaption(text string, width int) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, ansi.Strip(text))
	return ansi.Truncate(text, max(0, width), "")
}

func (v *Viewer) frame(size dimensions, native bool, id uint32, payload []byte) error {
	cols, rows := size.cols-1, size.rows-3 // Reserve last column to avoid autowrap.
	if cols < 1 || rows < 1 {
		return nil
	}
	if _, err := fmt.Fprintf(v.stdout, "%s\r\n", cleanCaption(v.caption, cols)); err != nil {
		return err
	}
	bounds := v.img.Bounds()
	if native {
		cellWidth := float64(size.width) / float64(size.cols)
		cellHeight := float64(size.height) / float64(size.rows)
		// Set only the limiting dimension: the protocol preserves aspect ratio.
		fit := fmt.Sprintf("c=%d", cols)
		if float64(bounds.Dy())*float64(cols)*cellWidth/float64(bounds.Dx()) > float64(rows)*cellHeight {
			fit = fmt.Sprintf("r=%d", rows)
		}
		if err := transmit(v.stdout, payload, id); err != nil {
			return err
		}
		if err := v.waitAck(id); err != nil {
			return err
		}
		if err := v.checkSize(size); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(v.stdout, "\x1b_Ga=p,i=%d,%s,C=1,q=0;\x1b\\", id, fit); err != nil {
			return err
		}
		if err := v.waitAck(id); err != nil {
			return err
		}
		if err := v.checkSize(size); err != nil {
			return err
		}
		_, err := fmt.Fprintf(v.stdout, "\x1b[%d;1H%s", size.rows, cleanCaption("Esc / q / Enter: return", cols))
		return err
	}
	cellAspect := 0.5
	if size.width > 0 && size.height > 0 {
		cellAspect = float64(size.width) * float64(size.rows) / (float64(size.height) * float64(size.cols))
	}
	width := cols
	height := max(1, int(math.Ceil(float64(width)*cellAspect*float64(bounds.Dy())/float64(bounds.Dx()))))
	if height > rows {
		height = rows
		width = max(1, int(float64(height)*float64(bounds.Dx())/(cellAspect*float64(bounds.Dy()))))
	}
	var line strings.Builder
	for y := range height {
		if err := v.ctx.Err(); err != nil {
			return err
		}
		line.Reset()
		for x := range width {
			sx := bounds.Min.X + x*bounds.Dx()/width
			top := bounds.Min.Y + (2*y)*bounds.Dy()/(2*height)
			bottom := bounds.Min.Y + (2*y+1)*bounds.Dy()/(2*height)
			r1, g1, b1, _ := v.img.At(sx, top).RGBA()
			r2, g2, b2, _ := v.img.At(sx, bottom).RGBA()
			fmt.Fprintf(&line, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", r1>>8, g1>>8, b1>>8, r2>>8, g2>>8, b2>>8)
		}
		line.WriteString("\x1b[0m\r\n")
		if _, err := io.WriteString(v.stdout, line.String()); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(v.stdout, cleanCaption("Esc / q / Enter: return", cols))
	return err
}

func transmit(w io.Writer, pngData []byte, id uint32) error {
	var encoded [4096]byte
	for offset := 0; offset < len(pngData); {
		end := min(offset+3072, len(pngData))
		n := base64.StdEncoding.EncodedLen(end - offset)
		base64.StdEncoding.Encode(encoded[:n], pngData[offset:end])
		more := 0
		if end < len(pngData) {
			more = 1
		}
		header := fmt.Sprintf("\x1b_Gm=%d;", more)
		if offset == 0 {
			header = fmt.Sprintf("\x1b_Gf=100,a=t,t=d,q=0,i=%d,m=%d;", id, more)
		}
		if _, err := io.WriteString(w, header); err != nil {
			return err
		}
		if nWritten, err := w.Write(encoded[:n]); err != nil {
			return err
		} else if nWritten != n {
			return io.ErrShortWrite
		}
		if _, err := io.WriteString(w, "\x1b\\"); err != nil {
			return err
		}
		offset = end
	}
	return nil
}

func deleteImage(w io.Writer, id uint32) error {
	_, err := fmt.Fprintf(w, "\x1b_Ga=d,d=I,i=%d,q=2;\x1b\\", id)
	return err
}
