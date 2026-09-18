package store

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

// 探测服务是否存在，不读取凭据，也不请求解锁钱包。
func systemKeyringMissing() (bool, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		var executable *exec.Error
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ENXIO) || errors.As(err, &executable) && errors.Is(executable.Err, exec.ErrNotFound) {
			return true, nil
		}
		return false, err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// ReadAlias 会激活已安装的服务，避免把尚未启动误判为不存在；
	// 此调用不会创建或解锁凭据集合。
	var collection dbus.ObjectPath
	err = conn.Object("org.freedesktop.secrets", "/org/freedesktop/secrets").CallWithContext(ctx, "org.freedesktop.Secret.Service.ReadAlias", 0, "default").Store(&collection)
	if err == nil {
		return false, nil
	}
	var busError dbus.Error
	if errors.As(err, &busError) {
		switch busError.Name {
		case "org.freedesktop.DBus.Error.ServiceUnknown", "org.freedesktop.DBus.Error.NameHasNoOwner":
			return true, nil
		}
	}
	return false, err
}
