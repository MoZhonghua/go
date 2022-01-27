import "fmt"

var x = ..

func init() { // do x }
func init() { // do y }


生成

func init() {
    x = ...
}

func init.0() { // do x }
func init.1() { // do 1 }


type InitTask struct {
    state uintptr
    depsLen uintptr
    fnsLen  uintptr

    deps []uintptr
    fns  []uintptr
}

var .inittask = InitTask {
    state: 0,
    depsLen: 1, // fmt.inittask
    fnsLen: 3,

    deps: []uintptr{fmt.inittask},
    fns: []uintptr{ init, init.0, init.1 }
}
