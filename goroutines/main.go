package main

import (
	"fmt"
	"math/rand"
	"runtime"
	"syscall"
	"time"
)

func aGoRoutine(routine int) {
	locked := false

	tabs := ""
	for i := 0; i < routine; i++ {
		tabs += "\t"
	}

	for {
		fmt.Printf("%sGo routine #%d in thread#%d\n", tabs, routine, syscall.Gettid())

		randInt := rand.Int63n(150)
		time.Sleep(time.Millisecond * time.Duration(randInt))

		randBool := bool(rand.Int63n(2) > 0)
		if randBool && locked {
			fmt.Printf("%sRoutine #%d will unlock the thread %d!\n", tabs, routine, syscall.Gettid())
			runtime.UnlockOSThread()
			locked = false
		} else if randBool {
			fmt.Printf("%sRoutine #%d will lock the thread %d!\n", tabs, routine, syscall.Gettid())
			runtime.LockOSThread()
			locked = true
		}
	}
}

func main() {
	runtime.GOMAXPROCS(1)

	go aGoRoutine(0)
	go aGoRoutine(1)
	go aGoRoutine(2)
	go aGoRoutine(3)

	fmt.Println("Hello world")
	select {}
}
