package listening

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/golang/glog"
)

type Listener interface {
	// Generics when?
	AddEvent(interface{})
	UpdateEvent(interface{}, interface{})
	DeleteEvent(interface{})
	Run(<-chan struct{})
}

// setup a signal hander to gracefully exit
func SigHandler() <-chan struct{} {
	stop := make(chan struct{})
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c,
			syscall.SIGINT,  // Ctrl+C
			syscall.SIGTERM, // Termination Request
			syscall.SIGSEGV, // FullDerp
			syscall.SIGABRT, // Abnormal termination
			syscall.SIGILL,  // illegal instruction
			syscall.SIGFPE)  // floating point - this is why we can't have nice things
		sig := <-c
		glog.Warningf("Signal (%v) Detected, Shutting Down", sig)
		close(stop)
	}()
	return stop
}

func RunAll(listeners []Listener) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	stop := SigHandler()
	for _, listener := range listeners {
		wg.Add(1)
		go func() {
			defer wg.Done()
			listener.Run(stop)
		}()
	}
	return wg
}
