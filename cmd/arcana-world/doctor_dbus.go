package main

import (
	"context"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// 仅接受本地 Unix 地址。SessionBus/ConnectSessionBus 可能自动启动总线；
// 调用 org.freedesktop.secrets 可能激活该服务。
// 这两种行为都不适用于诊断。
func doctorSecretService(ctx context.Context) (available, known bool, detail string) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	address := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if address == "" {
		root := os.Getenv("XDG_RUNTIME_DIR")
		if !filepath.IsAbs(root) {
			return false, false, "no explicit session bus address or absolute XDG_RUNTIME_DIR; autolaunch disabled"
		}
		path := filepath.Join(root, "bus")
		info, err := os.Stat(path)
		if err != nil || info.Mode()&os.ModeSocket == 0 {
			return false, false, "no existing session bus socket; autolaunch disabled"
		}
		address = "unix:path=" + url.PathEscape(path)
	}
	var socket string
	for _, candidate := range strings.Split(address, ";") {
		if !strings.HasPrefix(candidate, "unix:") {
			continue
		}
		for _, field := range strings.Split(strings.TrimPrefix(candidate, "unix:"), ",") {
			key, value, ok := strings.Cut(field, "=")
			if !ok || key != "path" && key != "abstract" {
				continue
			}
			decoded, err := url.PathUnescape(value)
			if err != nil || decoded == "" || strings.ContainsRune(decoded, '\x00') {
				continue
			}
			if key == "abstract" {
				socket = "\x00" + decoded
			} else if filepath.IsAbs(decoded) {
				socket = decoded
			}
			if socket != "" {
				break
			}
		}
		if socket != "" {
			break
		}
	}
	if socket == "" {
		return false, false, "session bus address has no supported local Unix endpoint; no connection attempted"
	}
	raw, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return false, false, "existing session bus is unreachable; autolaunch disabled"
	}
	defer raw.Close()
	// 截止时间和取消操作同时覆盖 Auth/Hello 以及方法调用。
	if deadline, ok := ctx.Deadline(); ok {
		_ = raw.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = raw.Close() })
	defer stop()
	conn, err := dbus.NewConn(raw, dbus.WithContext(ctx))
	if err != nil {
		return false, false, "cannot create private session bus connection"
	}
	defer conn.Close()
	if err = conn.Auth(nil); err != nil {
		return false, false, "session bus authentication failed; credentials were not accessed"
	}
	if err = conn.Hello(); err != nil {
		return false, false, "session bus handshake failed"
	}
	bus := conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus")
	var owned bool
	if err = bus.CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", dbus.FlagNoAutoStart, "org.freedesktop.secrets").Store(&owned); err != nil {
		return false, false, "cannot inspect Secret Service owner"
	}
	if owned {
		return true, true, "Secret Service already owns its bus name (service not contacted)"
	}
	var names []string
	if err = bus.CallWithContext(ctx, "org.freedesktop.DBus.ListActivatableNames", dbus.FlagNoAutoStart).Store(&names); err != nil {
		return false, false, "cannot inspect activatable session bus names"
	}
	if slices.Contains(names, "org.freedesktop.secrets") {
		return true, true, "Secret Service is registered for activation (not activated)"
	}
	return false, true, "Secret Service has no owner or activation entry"
}
