package main

import (
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"
)

var (
	heapAddrBits    = uintptr(48)
	arenaBaseOffset = uintptr(0xffff800000000000)
	heapArenaBytes  = uintptr(67108864)

	t    = uintptr(1024 * 1024 * 1024 * 1024)
	t128 = uintptr(128 * t)
)

func arenaIndex(p uintptr) arenaIdx {
	return arenaIdx((p - arenaBaseOffset) / heapArenaBytes)
}

// arenaBase returns the low address of the region covered by heap
// arena i.
func arenaBase(i arenaIdx) uintptr {
	return uintptr(i)*heapArenaBytes + arenaBaseOffset
}

type arenaIdx uint

func main() {
	maxOffAddr := (((1 << heapAddrBits) - 1) + arenaBaseOffset)
	fmt.Printf("%x\n", maxOffAddr)
	fmt.Printf("%x\n", (1<<heapAddrBits)-1)
	fmt.Printf("%x\n", ^uintptr(((1 << 47) - 1)))

	x := 0
	fmt.Printf("%p\n", &x)

	z := uintptr(unsafe.Pointer(&x))
	_ = z
	fmt.Printf("arenaIdx2: 0x%x -> %v\n", z, z/heapArenaBytes)
	fmt.Printf("arenaIdx : 0x%x -> %v\n", z-arenaBaseOffset, arenaIndex(z))

	z = t128 + z
	fmt.Printf("arenaIdx2: 0x%x -> %v\n", z, z/heapArenaBytes)
	fmt.Printf("arenaIdx : 0x%x -> %v\n", z-arenaBaseOffset, arenaIndex(z))
	os.Exit(0)

	fmt.Println("==============")
	for i := uintptr(0); i <= 256*t; i += 16 * t {
		fmt.Printf("arenaIdx : %3d -> %v\n", i/t, arenaIndex(i))
	}


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
