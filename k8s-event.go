package main

import (
	"flag"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/AeroNotix/k8s-event/pkg/listening"

	"github.com/golang/glog"
	"github.com/spf13/viper"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// setup a signal hander to gracefully exit
func sigHandler() <-chan struct{} {
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

func loadConfig() kubernetes.Interface {
	flag.Parse()
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	return clientset
}

func main() {
	clientset := loadConfig()
	sharedInformers := informers.NewSharedInformerFactory(clientset, viper.GetDuration("resync-interval"))
	podsInformer := sharedInformers.Core().V1().Pods()
	stop := sigHandler()
	peh := listener.NewPodEventHandler(clientset, podsInformer)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		peh.Run(stop)
	}()
	glog.Infof("Starting shared Informer(s)")
	sharedInformers.Start(stop)
	wg.Wait()
	glog.Warningf("Exiting main()")
	os.Exit(1)

}
