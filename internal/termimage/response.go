package termimage

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

var errPreviewClosed = errors.New("preview closed")
var errPreviewResized = errors.New("preview resized")

// Acknowledging upload before placement gives the terminal a completed image
// resource. Acknowledging placement before drawing the footer commits the frame
// without sleeps or periodic redraws while the terminal decodes the image.
func (v *Viewer) waitAck(id uint32) error {
	in, ok := v.stdin.(*os.File)
	if !ok {
		return errors.New("preview: graphics acknowledgments require terminal input")
	}
	deadline := time.Now().Add(3 * time.Second)
	var parser responseParser
	var input [256]byte
	for time.Now().Before(deadline) {
		if err := v.ctx.Err(); err != nil {
			return err
		}
		n, err := pollInput(in, input[:])
		if err != nil {
			return err
		}
		if n == 0 && len(parser.packet) == 1 {
			return errPreviewClosed
		}
		done, err := parser.feed(input[:n], id)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	return errors.New("preview: terminal did not acknowledge image within 3 seconds")
}

func (v *Viewer) checkSize(expected dimensions) error {
	out, ok := v.stdout.(*os.File)
	if !ok {
		return errors.New("preview: native graphics require terminal output")
	}
	actual, err := terminalSize(out)
	if err != nil {
		return err
	}
	if actual != expected {
		return errPreviewResized
	}
	return nil
}

// Control sequences may be split at any byte boundary, including ESC. Keep
// them separate from keyboard input, so a terminal ACK never closes the viewer.
// Only a bounded packet and responses for our own image are accepted.
type responseParser struct{ packet []byte }

func (p *responseParser) feed(data []byte, id uint32) (bool, error) {
	done := false
	for _, c := range data {
		if len(p.packet) == 0 {
			switch c {
			case 27:
				p.packet = append(p.packet, c)
			case 'q', 'Q', '\r', '\n', 3:
				return false, errPreviewClosed
			}
			continue
		}
		if len(p.packet) == 1 && c != '_' {
			return false, errPreviewClosed
		}
		if len(p.packet) == 2 && c != 'G' {
			return false, errPreviewClosed
		}
		p.packet = append(p.packet, c)
		if len(p.packet) > 2048 {
			return false, errors.New("preview: oversized terminal graphics response")
		}
		if !bytes.HasSuffix(p.packet, []byte{'\x1b', '\\'}) {
			continue
		}
		header, status, found := strings.Cut(string(p.packet[3:len(p.packet)-2]), ";")
		p.packet = p.packet[:0]
		if !found {
			continue
		}
		for _, field := range strings.Split(header, ",") {
			key, value, found := strings.Cut(field, "=")
			if !found || key != "i" {
				continue
			}
			responseID, err := strconv.ParseUint(value, 10, 32)
			if err != nil || uint32(responseID) != id {
				continue
			}
			if status != "OK" {
				return false, fmt.Errorf("preview: terminal rejected image: %s", cleanCaption(status, 160))
			}
			done = true
		}
	}
	return done, nil
}
