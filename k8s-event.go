package main

/* Some code samples taken from https://github.com/heptiolabs/eventrouter */

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/golang/glog"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// EventRouter is responsible for maintaining a stream of kubernetes
// system Events and pushing them to another channel for storage
type EventRouter struct {
	// kubeclient is the main kubernetes interface
	kubeClient kubernetes.Interface

	// store of events populated by the shared informer
	eLister corelisters.EventLister

	// returns true if the event store has been synced
	eListerSynched cache.InformerSynced
}

type EventData struct {
	Verb     string    `json:"verb"`
	Event    *v1.Event `json:"event"`
	OldEvent *v1.Event `json:"old_event,omitempty"`
}

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

func NewEventData(eNew *v1.Event, eOld *v1.Event) EventData {
	var eData EventData
	if eOld == nil {
		eData = EventData{
			Verb:  "ADDED",
			Event: eNew,
		}
	} else {
		eData = EventData{
			Verb:     "UPDATED",
			Event:    eNew,
			OldEvent: eOld,
		}
	}

	return eData
}

// UpdateEvents implements the EventSinkInterface
func UpdateEvents(eNew *v1.Event, eOld *v1.Event) {
	eData := NewEventData(eNew, eOld)
	log.Printf("%s - %s - %s - %s", eData.Event.Reason, eData.Event.InvolvedObject.Namespace, eData.Event.InvolvedObject.Name, eData.Event.Message)
	if eData.Event.Related != nil {
		log.Printf("%s - %s", eData.Event.Related.Namespace, eData.Event.Related.Name)
	}
}

// addEvent is called when an event is created, or during the initial list
func addEvent(obj interface{}) {
	e := obj.(*v1.Event)
	UpdateEvents(e, nil)
}

// updateEvent is called any time there is an update to an existing event
func updateEvent(objOld interface{}, objNew interface{}) {
	eOld := objOld.(*v1.Event)
	eNew := objNew.(*v1.Event)
	UpdateEvents(eNew, eOld)
}

// deleteEvent should only occur when the system garbage collects events via TTL expiration
func deleteEvent(obj interface{}) {
	e := obj.(*v1.Event)
	// NOTE: This should *only* happen on TTL expiration there
	// is no reason to push this to a sink
	glog.V(5).Infof("Event Deleted from the system:\n%v", e)
}

func NewEventRouter(kubeClient kubernetes.Interface, eventsInformer coreinformers.EventInformer) *EventRouter {
	er := &EventRouter{}
	eventsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    addEvent,
		UpdateFunc: updateEvent,
		DeleteFunc: deleteEvent,
	})
	er.eLister = eventsInformer.Lister()
	er.eListerSynched = eventsInformer.Informer().HasSynced
	return er
}

func (er *EventRouter) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer glog.Infof("Shutting down EventRouter")

	glog.Infof("Starting EventRouter")

	if !cache.WaitForCacheSync(stopCh, er.eListerSynched) {
		utilruntime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}
	<-stopCh
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
	eventsInformer := sharedInformers.Core().V1().Events()
	eventRouter := NewEventRouter(clientset, eventsInformer)
	stop := sigHandler()
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		eventRouter.Run(stop)
	}()
	glog.Infof("Starting shared Informer(s)")
	sharedInformers.Start(stop)
	wg.Wait()
	glog.Warningf("Exiting main()")
	os.Exit(1)

}
