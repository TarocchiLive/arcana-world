//go:build windows && 386

package native

import (
	"fmt"
	"math"
	"syscall"
	"unsafe"
)

// 386 的 COM 方法使用 stdcall；float32 按原始位占一个栈槽，POINT_2F 按值占两个。
// 只适配绘制实际使用的签名，新增签名必须在此显式描述，不能退回通用反射调用。
// 指针在 SyscallN 实参中直接转换，确保编译器将其保活并固定到调用结束。
func winRegisterMethod(method any, address uintptr) {
	switch method := method.(type) {
	case *func(*winCOMObject, *uint16, *winCOMObject, uint32, uint32, uint32, float32, *uint16, **winCOMObject) int32:
		*method = func(object *winCOMObject, family *uint16, collection *winCOMObject, weight, style, stretch uint32, size float32, locale *uint16, result **winCOMObject) int32 {
			hr, _, _ := syscall.SyscallN(address, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(family)), uintptr(unsafe.Pointer(collection)), uintptr(weight), uintptr(style), uintptr(stretch), uintptr(math.Float32bits(size)), uintptr(unsafe.Pointer(locale)), uintptr(unsafe.Pointer(result)))
			return int32(hr)
		}
	case *func(*winCOMObject, *uint16, uint32, *winCOMObject, float32, float32, **winCOMObject) int32:
		*method = func(object *winCOMObject, text *uint16, length uint32, format *winCOMObject, width, height float32, result **winCOMObject) int32 {
			hr, _, _ := syscall.SyscallN(address, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(text)), uintptr(length), uintptr(unsafe.Pointer(format)), uintptr(math.Float32bits(width)), uintptr(math.Float32bits(height)), uintptr(unsafe.Pointer(result)))
			return int32(hr)
		}
	case *func(*winCOMObject, uint32, float32, float32) int32:
		*method = func(object *winCOMObject, method uint32, spacing, baseline float32) int32 {
			hr, _, _ := syscall.SyscallN(address, uintptr(unsafe.Pointer(object)), uintptr(method), uintptr(math.Float32bits(spacing)), uintptr(math.Float32bits(baseline)))
			return int32(hr)
		}
	case *func(*winCOMObject, uint32) int32:
		*method = func(object *winCOMObject, value uint32) int32 {
			hr, _, _ := syscall.SyscallN(address, uintptr(unsafe.Pointer(object)), uintptr(value))
			return int32(hr)
		}
	case *func(*winCOMObject, *winLineMetrics, uint32, *uint32) int32:
		*method = func(object *winCOMObject, metrics *winLineMetrics, capacity uint32, count *uint32) int32 {
			hr, _, _ := syscall.SyscallN(address, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(metrics)), uintptr(capacity), uintptr(unsafe.Pointer(count)))
			return int32(hr)
		}
	case *func(*winCOMObject, *winCOMObject, winTextRange) int32:
		*method = func(object, effect *winCOMObject, span winTextRange) int32 {
			hr, _, _ := syscall.SyscallN(address, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(effect)), uintptr(span.Start), uintptr(span.Length))
			return int32(hr)
		}
	case *func(*winCOMObject, *winD2DProperties, **winCOMObject) int32:
		*method = func(object *winCOMObject, properties *winD2DProperties, result **winCOMObject) int32 {
			hr, _, _ := syscall.SyscallN(address, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(properties)), uintptr(unsafe.Pointer(result)))
			return int32(hr)
		}
	case *func(*winCOMObject, uintptr, *winRect) int32:
		*method = func(object *winCOMObject, dc uintptr, rect *winRect) int32 {
			hr, _, _ := syscall.SyscallN(address, uintptr(unsafe.Pointer(object)), dc, uintptr(unsafe.Pointer(rect)))
			return int32(hr)
		}
	case *func(*winCOMObject, *[4]float32, unsafe.Pointer, **winCOMObject) int32:
		*method = func(object *winCOMObject, color *[4]float32, properties unsafe.Pointer, result **winCOMObject) int32 {
			hr, _, _ := syscall.SyscallN(address, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(color)), uintptr(properties), uintptr(unsafe.Pointer(result)))
			return int32(hr)
		}
	case *func(*winCOMObject, uint32):
		*method = func(object *winCOMObject, value uint32) {
			syscall.SyscallN(address, uintptr(unsafe.Pointer(object)), uintptr(value))
		}
	case *func(*winCOMObject, *[6]float32):
		*method = func(object *winCOMObject, matrix *[6]float32) {
			syscall.SyscallN(address, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(matrix)))
		}
	case *func(*winCOMObject):
		*method = func(object *winCOMObject) {
			syscall.SyscallN(address, uintptr(unsafe.Pointer(object)))
		}
	case *func(*winCOMObject, *[4]float32):
		*method = func(object *winCOMObject, color *[4]float32) {
			syscall.SyscallN(address, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(color)))
		}
	case *func(*winCOMObject, *[4]float32, uint32):
		*method = func(object *winCOMObject, rect *[4]float32, mode uint32) {
			syscall.SyscallN(address, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(rect)), uintptr(mode))
		}
	case *func(*winCOMObject, winPointF, *winCOMObject, *winCOMObject, uint32):
		*method = func(object *winCOMObject, origin winPointF, layout, brush *winCOMObject, options uint32) {
			syscall.SyscallN(address, uintptr(unsafe.Pointer(object)), uintptr(math.Float32bits(origin.X)), uintptr(math.Float32bits(origin.Y)), uintptr(unsafe.Pointer(layout)), uintptr(unsafe.Pointer(brush)), uintptr(options))
		}
	case *func(*winCOMObject, *uint64, *uint64) int32:
		*method = func(object *winCOMObject, tag1, tag2 *uint64) int32 {
			hr, _, _ := syscall.SyscallN(address, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(tag1)), uintptr(unsafe.Pointer(tag2)))
			return int32(hr)
		}
	default:
		panic(fmt.Sprintf("overlay: unsupported Windows 386 COM method signature %T", method))
	}
}
