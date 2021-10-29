package main

import (
	"fmt"
	"sync"
	"time"
	"unsafe"
)

func main() {
	heapAddrBits:= uintptr(48)
	arenaBaseOffset:= uintptr(0xffff800000000000)

	maxOffAddr := (((1 << heapAddrBits) - 1) + arenaBaseOffset)
	fmt.Printf("%x\n", maxOffAddr)
	fmt.Printf("%x\n", (1 << heapAddrBits) - 1);
	fmt.Printf("%x\n", ^uintptr(((1 << 47) - 1)));


	x := 0;
	fmt.Printf("%p\n", &x)

	z := uintptr(unsafe.Pointer(&x))
	fmt.Printf("0x%x\n", z - arenaBaseOffset)

	var lock sync.Mutex
	go func() {
		lock.Lock()
		time.Sleep(time.Second * 100000)
		lock.Unlock()
	}()

	time.Sleep(time.Millisecond * 10)
	lock.Lock()
	lock.Unlock()
}
