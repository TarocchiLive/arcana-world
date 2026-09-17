package termimage

import (
	"arcana-world/internal/i18n"

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

const graphicsAckTimeout = 3 * time.Second

func (v *Viewer) waitPlacementReady(id uint32, size dimensions) error {
	if err := v.waitAck(id); err != nil {
		return err
	}
	return v.checkSize(size)
}

// 放置前等待上传确认，确保终端已有完整图像资源；
// 绘制页脚前等待放置确认，确保画面提交完成，
// 无需在终端解码图片时通过休眠或周期性重绘等待。
func (v *Viewer) waitAck(id uint32) error {
	in, ok := v.stdin.(*os.File)
	if !ok {
		return errors.New(i18n.T(i18n.TermImageAcknowledgmentInputRequired))
	}
	deadline := time.Now().Add(graphicsAckTimeout)
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
	return errors.New(i18n.T(i18n.TermImageAcknowledgmentTimeout))
}

func (v *Viewer) checkSize(expected dimensions) error {
	out, ok := v.stdout.(*os.File)
	if !ok {
		return errors.New(i18n.T(i18n.TermImageNativeOutputRequired))
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

// 控制序列可能在任意字节边界拆分，包括 ESC；必须将其与键盘输入
// 分开处理，避免终端确认响应导致查看器关闭。
// 仅接受大小受限的数据包，以及属于本查看器图像的响应。
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
			return false, errors.New(i18n.T(i18n.TermImageResponseTooLarge))
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
				return false, fmt.Errorf(i18n.T(i18n.TermImageImageRejected), cleanCaption(status, 160))
			}
			done = true
		}
	}
	return done, nil
}
