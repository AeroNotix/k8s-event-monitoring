package healthchecks

import (
	"fmt"
	"strings"

	"github.com/AeroNotix/k8s-event/pkg/alerting"
	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type LivenessFailedHandler struct {
	// kubeclient is the main kubernetes interface
	kubeClient kubernetes.Interface
	// store of events populated by the shared informer
	eLister corelisters.EventLister
	// returns true if the event store has been synced
	eListerSynched cache.InformerSynced
	// The alerter we send events to
	alerter alerting.Alerter
}

func isKillingBecauseHealthchecksFailed(msg string) bool {
	return strings.Contains(msg, "failed liveness probe")
}

func (lfh LivenessFailedHandler) handleEvent(obj interface{}) {
	event, ok := obj.(*v1.Event)
	if ok && event.Reason == "Killing" && isKillingBecauseHealthchecksFailed(event.Message) {
		lfh.alerter.Alert(alerting.ContainerRestartEvent{})
	}
}

func (lfh LivenessFailedHandler) AddEvent(obj interface{}) {
	lfh.handleEvent(obj)
}

func (lfh LivenessFailedHandler) UpdateEvent(objOld interface{}, objNew interface{}) {
	lfh.handleEvent(objNew)
}

func (lfh LivenessFailedHandler) DeleteEvent(interface{}) {}

func New(kubeClient kubernetes.Interface, informerFactory informers.SharedInformerFactory, alerter alerting.Alerter) *LivenessFailedHandler {
	eventsInformer := informerFactory.Core().V1().Events()
	lfh := &LivenessFailedHandler{
		kubeClient: kubeClient,
		alerter:    alerter,
	}
	eventsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    lfh.AddEvent,
		UpdateFunc: lfh.UpdateEvent,
		DeleteFunc: lfh.DeleteEvent,
	})
	lfh.eLister = eventsInformer.Lister()
	lfh.eListerSynched = eventsInformer.Informer().HasSynced
	return lfh
}

func (er *LivenessFailedHandler) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	if !cache.WaitForCacheSync(stopCh, er.eListerSynched) {
		utilruntime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}
	<-stopCh
}
