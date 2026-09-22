package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
	"github.com/skip2/go-qrcode"
)

type qrImageOpened struct{ err error }

func (m *Model) prepareQR(link string) tea.Cmd {
	m.qrText = ""
	if preferQRImage() {
		return m.openQRImage()
	}
	m.qrText = renderQR(link)
	return nil
}

func (m *Model) openQRImage() tea.Cmd {
	if m.qrImageOpening {
		return nil
	}
	link := m.faceURL
	if m.mode == "qr" {
		if m.qr == nil {
			return nil
		}
		link = m.qr.URL
	}
	if link == "" {
		return nil
	}
	m.qrImageOpening = true
	ctx := m.ctx
	return func() tea.Msg {
		return qrImageOpened{err: openQRBitmap(ctx, link)}
	}
}

func openQRBitmap(ctx context.Context, link string) error {
	qr, err := qrcode.New(link, qrcode.Low)
	if err != nil {
		return err
	}
	// 整数倍缩放可保持码点清晰，并保留默认静区。
	data, err := qr.PNG(-8)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp("", "arcana-world-qr-*.png")
	if err != nil {
		return err
	}
	path := file.Name()
	if _, err := file.Write(data); err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", path)
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32.exe", "url.dll,FileProtocolHandler", path)
	case "linux", "freebsd", "openbsd", "netbsd":
		cmd = exec.CommandContext(ctx, "xdg-open", path)
	default:
		os.Remove(path)
		return fmt.Errorf("opening images is unsupported on %s", runtime.GOOS)
	}
	if err := cmd.Run(); err != nil {
		os.Remove(path)
		return err
	}
	// 查看器可能异步读取，图片文件保留到程序退出。
	context.AfterFunc(ctx, func() { _ = os.Remove(path) })
	return nil
}
