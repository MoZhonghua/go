package runtime

import "unsafe"

type markdebugdata struct {
	g       unsafe.Pointer
	g0      unsafe.Pointer
	gsignal unsafe.Pointer
	obj     uintptr
}

func (d *markdebugdata) needlog(gp *g) bool {
	if d.g != nil && d.g == unsafe.Pointer(gp) {
		return true
	}

	if d.g0 != nil && d.g0 == unsafe.Pointer(gp) {
		return true
	}

	if d.gsignal != nil && d.gsignal == unsafe.Pointer(gp) {
		return true
	}

	return false
}

var markdebug markdebugdata

func SetMarkDebug(obj uintptr) {
	g := getg()
	markdebug.g = unsafe.Pointer(g)
	if g.m != nil {
		markdebug.g0 = unsafe.Pointer(g.m.g0)
		markdebug.gsignal = unsafe.Pointer(g.m.gsignal)
	}
	markdebug.obj = obj
}

func Getg() unsafe.Pointer {
	return unsafe.Pointer(getg())
}
