package rtmpsrt

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"strconv"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// 库句柄保留至进程退出，避免并发连接仍在调用已卸载的原生代码。
type nativeAPI struct {
	startup func() int32
	socket  func() int32
	option  func(int32, int32, unsafe.Pointer, int32) int32
	connect func(int32, unsafe.Pointer, int32) int32
	send    func(int32, unsafe.Pointer, int32, int32, int32) int32
	recv    func(int32, unsafe.Pointer, int32) int32
	close   func(int32) int32
}

var library struct {
	sync.Mutex
	api *nativeAPI
}

func loadSRT() (*nativeAPI, error) {
	library.Lock()
	defer library.Unlock()
	if library.api != nil {
		return library.api, nil
	}
	lookup, err := openSRT()
	if err != nil {
		return nil, err
	}
	a := &nativeAPI{}
	for _, item := range []struct {
		name string
		fn   any
	}{
		{"srt_startup", &a.startup}, {"srt_create_socket", &a.socket}, {"srt_setsockflag", &a.option},
		{"srt_connect", &a.connect}, {"srt_sendmsg", &a.send}, {"srt_recvmsg", &a.recv}, {"srt_close", &a.close},
	} {
		address, err := lookup(item.name)
		if err != nil {
			return nil, fmt.Errorf("libsrt: missing %s", item.name)
		}
		purego.RegisterFunc(item.fn, address)
	}
	if a.startup() != 0 {
		return nil, errors.New("libsrt initialization failed")
	}
	library.api = a
	return a, nil
}

type srtConn struct {
	api     *nativeAPI
	socket  int32
	once    sync.Once
	pending []byte
	buffer  [1456]byte
}

func (c *srtConn) Close() error { c.once.Do(func() { c.api.close(c.socket) }); return nil }
func (c *srtConn) Write(p []byte) (int, error) {
	total := 0
	for len(p) > 0 {
		n := min(len(p), 1424)
		sent := int(c.api.send(c.socket, unsafe.Pointer(&p[0]), int32(n), -1, 1))
		runtime.KeepAlive(p)
		if sent != n {
			return total, errors.New("SRT send failed")
		}
		total += n
		p = p[n:]
	}
	return total, nil
}
func (c *srtConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(c.pending) == 0 {
		n := int(c.api.recv(c.socket, unsafe.Pointer(&c.buffer[0]), int32(len(c.buffer))))
		if n <= 0 {
			return 0, io.EOF
		}
		c.pending = c.buffer[:n]
	}
	n := copy(p, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}

func dialSRT(ctx context.Context, a *nativeAPI, target Target) (*srtConn, error) {
	host, port, _ := net.SplitHostPort(target.Host)
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return nil, errors.New("SRT host lookup failed")
	}
	if len(ips) == 0 {
		return nil, errors.New("SRT host has no IPv4 address")
	}
	id := a.socket()
	if id < 0 {
		return nil, errors.New("SRT socket creation failed")
	}
	c := &srtConn{api: a, socket: id}
	ok := false
	defer func() {
		if !ok {
			c.Close()
		}
	}()
	// 与参考 processor 分支一致：LIVE、消息模式、1456 载荷、200ms 延迟；不启用时间戳交付/迟包丢弃。
	for _, option := range [][2]int32{{50, 0}, {48, 1}, {49, 1456}, {23, 200}, {31, 0}, {22, 0}, {36, 10000}, {1, 1}, {2, 1}} {
		value := option[1]
		if a.option(id, option[0], unsafe.Pointer(&value), 4) != 0 {
			return nil, fmt.Errorf("SRT option %d unavailable", option[0])
		}
	}
	// Windows 的 linger 为两个 uint16；Unix 为两个 int32，零值均表示立即关闭。
	var linger [8]byte
	lingerSize := int32(len(linger))
	if runtime.GOOS == "windows" {
		lingerSize = 4
	}
	if a.option(id, 7, unsafe.Pointer(&linger[0]), lingerSize) != 0 {
		return nil, errors.New("SRT linger configuration failed")
	}
	var addr [16]byte
	if runtime.GOOS == "darwin" {
		addr[0] = 16
		addr[1] = 2
	} else {
		binary.NativeEndian.PutUint16(addr[:2], 2)
	}
	portNumber, _ := strconv.Atoi(port)
	binary.BigEndian.PutUint16(addr[2:4], uint16(portNumber))
	copy(addr[4:8], ips[0].To4())
	cancelDone := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { c.Close(); close(cancelDone) })
	result := a.connect(id, unsafe.Pointer(&addr[0]), 16)
	if !stop() {
		<-cancelDone
		return nil, ctx.Err()
	}
	if result != 0 {
		return nil, errors.New("SRT connection failed")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ok = true
	return c, nil
}
