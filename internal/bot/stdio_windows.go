package bot

import (
	"io"
	"os"
)

// Stdio 使用 Go 原生标准流；Windows 管道的 Close 会取消阻塞中的 I/O。
// 标准流由机器人模式独占，调用方应在退出时关闭。
func Stdio() (io.ReadCloser, io.WriteCloser, error) {
	return os.Stdin, os.Stdout, nil
}
