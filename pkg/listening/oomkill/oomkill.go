package oomkill

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/AeroNotix/k8s-event/pkg/alerting"
	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const OOMKILL = "OOMKilled"

type PodEventHandler struct {
	// kubeclient is the main kubernetes interface
	kubeClient kubernetes.Interface
	// store of events populated by the shared informer
	eLister corelisters.PodLister
	// returns true if the event store has been synced
	eListerSynched cache.InformerSynced
	// The alerter we send events to
	alerter alerting.Alerter
}

type ContainerStatusMap map[string]v1.ContainerStatus

func (peh PodEventHandler) AddEvent(obj interface{}) {
	// We don't need to do anything with this.
}

func containerStatusesToMap(cs []v1.ContainerStatus) ContainerStatusMap {
	csMap := make(ContainerStatusMap, len(cs))
	for _, cs := range cs {
		csMap[cs.Name] = cs
	}
	return csMap
}

func findOOMKilledContainers(csmOld, csmNew ContainerStatusMap) []v1.ContainerStatus {
	var restartedContainers []v1.ContainerStatus
	for _, csOld := range csmOld {
		csNew, ok := csmNew[csOld.Name]
		if ok && csNew.RestartCount > csOld.RestartCount && csNew.LastTerminationState.Terminated.Reason == OOMKILL {
			restartedContainers = append(restartedContainers, csNew)
		}
	}
	return restartedContainers
}

func (peh PodEventHandler) getPreviousPodLogs(pod *v1.Pod, containerName string) ([]byte, error) {
	tail := int64(100)
	podLogOpts := v1.PodLogOptions{
		Previous:  true,
		TailLines: &tail,
		Container: containerName,
	}
	req := peh.kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOpts)
	podLogs, err := req.Stream()
	if err != nil {
		return nil, err
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return nil, err
	}
	str := buf.Bytes()
	return str, nil
}

func (peh PodEventHandler) formatPodRestartMessage(pod *v1.Pod, cs v1.ContainerStatus) error {
	// v1.ObjectMeta.ClusterName isn't set properly in kubernetes:
	// https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#ObjectMeta,
	// because this runs in each cluster, we can do this:
	clusterName := os.Getenv("CLUSTER_NAME")
	previousLogs, err := peh.getPreviousPodLogs(pod, cs.Name)
	if err != nil {
		return err
	}
	cre := alerting.ContainerRestartEvent{
		ContainerName: cs.Name,
		ClusterName:   clusterName,
		PodName:       pod.Name,
		Namespace:     pod.Namespace,
		Reason:        cs.LastTerminationState.Terminated.Reason,
		LastLogs:      string(previousLogs),
	}
	return peh.alerter.Alert(cre)
}

func (peh PodEventHandler) UpdateEvent(objOld interface{}, objNew interface{}) {
	eOld, ok0 := objOld.(*v1.Pod)
	eNew, ok1 := objNew.(*v1.Pod)
	if !ok0 || !ok1 {
		return
	}
	containerStatusesOld := containerStatusesToMap(eOld.Status.ContainerStatuses)
	containerStatusesNew := containerStatusesToMap(eNew.Status.ContainerStatuses)
	restartedContainers := findOOMKilledContainers(containerStatusesOld, containerStatusesNew)
	for _, restartedContainer := range restartedContainers {
		err := peh.formatPodRestartMessage(eNew, restartedContainer)
		if err != nil {
			log.Println("error formatting pod restart message logs: %s", err)
			continue
		}
	}
}

func (peh PodEventHandler) DeleteEvent(obj interface{}) {
	// We don't need to do anything with this.
}

func New(kubeClient kubernetes.Interface, informerFactory informers.SharedInformerFactory, alerter alerting.Alerter) *PodEventHandler {
	podsInformer := informerFactory.Core().V1().Pods()
	peh := &PodEventHandler{
		kubeClient: kubeClient,
		alerter:    alerter,
	}
	podsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    peh.AddEvent,
		UpdateFunc: peh.UpdateEvent,
		DeleteFunc: peh.DeleteEvent,
	})
	peh.eLister = podsInformer.Lister()
	peh.eListerSynched = podsInformer.Informer().HasSynced
	return peh
}

func (er *PodEventHandler) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	if !cache.WaitForCacheSync(stopCh, er.eListerSynched) {
		utilruntime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}
	<-stopCh
}
