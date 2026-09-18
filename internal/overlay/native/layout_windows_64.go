//go:build windows && (amd64 || arm64)

package native

import "github.com/ebitengine/purego"

func winRegisterMethod(method any, address uintptr) {
	purego.RegisterFunc(method, address)
}
