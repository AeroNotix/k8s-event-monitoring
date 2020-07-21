package listener

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"text/template"

	"github.com/ashwanthkumar/slack-go-webhook"
	"github.com/golang/glog"
	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type PodEventHandler struct {
	// kubeclient is the main kubernetes interface
	kubeClient kubernetes.Interface
	// store of events populated by the shared informer
	eLister corelisters.PodLister
	// returns true if the event store has been synced
	eListerSynched cache.InformerSynced
}

type ContainerStatusMap map[string]v1.ContainerStatus

type ContainerRestartMessage struct {
	ContainerName string
	ClusterName   string
	PodName       string
	Namespace     string
	Reason        string
	LastLogs      string
}

func (peh PodEventHandler) addEvent(obj interface{}) {
	// We don't need to do anything with this.
}

func containerStatusesToMap(cs []v1.ContainerStatus) ContainerStatusMap {
	csMap := make(ContainerStatusMap, len(cs))
	for _, cs := range cs {
		csMap[cs.Name] = cs
	}
	return csMap
}

// SendMessage ... sends a message to slack?
func SendMessage(webhookURL, channel, message string) {
	payload := slack.Payload{
		Text:      message,
		Username:  "K8sEventMonitoring",
		Channel:   channel,
		IconEmoji: ":skull_and_crossbones:",
	}
	err := slack.Send(webhookURL, "", payload)
	if len(err) > 0 {
		log.Printf("error: %s\n", err)
	}
}

func findRestartedContainers(csmOld, csmNew ContainerStatusMap) []v1.ContainerStatus {
	var restartedContainers []v1.ContainerStatus
	for _, csOld := range csmOld {
		csNew, ok := csmNew[csOld.Name]
		if ok && csNew.RestartCount > csOld.RestartCount {
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
	podRestartTemplate := `
Container {{.PodName}}/{{.ContainerName}} restarted in {{.ClusterName}}/{{.Namespace}}
* Reason: {{.Reason}}
* Logs:` + "\n```{{.LastLogs}}```"

	tmpl, err := template.New("restartTemplate").Parse(podRestartTemplate)
	if err != nil {
		return err
	}
	previousLogs, err := peh.getPreviousPodLogs(pod, cs.Name)
	if err != nil {
		return err
	}
	crm := ContainerRestartMessage{
		ContainerName: cs.Name,
		ClusterName:   pod.ClusterName,
		PodName:       pod.Name,
		Namespace:     pod.Namespace,
		Reason:        cs.LastTerminationState.Terminated.Reason,
		LastLogs:      string(previousLogs),
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, crm)
	if err != nil {
		return err
	}
	SendMessage(os.Getenv("KW_SLACKWH_URI"), os.Getenv("KW_SLACKWH_CHANNEL"), buf.String())
	return nil
}

func (peh PodEventHandler) updateEvent(objOld interface{}, objNew interface{}) {
	eOld := objOld.(*v1.Pod)
	eNew := objNew.(*v1.Pod)
	containerStatusesOld := containerStatusesToMap(eOld.Status.ContainerStatuses)
	containerStatusesNew := containerStatusesToMap(eNew.Status.ContainerStatuses)
	restartedContainers := findRestartedContainers(containerStatusesOld, containerStatusesNew)
	for _, restartedContainer := range restartedContainers {
		err := peh.formatPodRestartMessage(eNew, restartedContainer)
		if err != nil {
			log.Println("error formatting pod restart message logs: %s", err)
			continue
		}
	}
}

func (peh PodEventHandler) deleteEvent(obj interface{}) {
	// We don't need to do anything with this.
}

func NewPodEventHandler(kubeClient kubernetes.Interface, eventsInformer coreinformers.PodInformer) *PodEventHandler {
	peh := &PodEventHandler{
		kubeClient: kubeClient,
	}
	eventsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    peh.addEvent,
		UpdateFunc: peh.updateEvent,
		DeleteFunc: peh.deleteEvent,
	})
	peh.eLister = eventsInformer.Lister()
	peh.eListerSynched = eventsInformer.Informer().HasSynced
	return peh
}

func (er *PodEventHandler) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer glog.Infof("Shutting down EventRouter")

	glog.Infof("Starting EventRouter")

	if !cache.WaitForCacheSync(stopCh, er.eListerSynched) {
		utilruntime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}
	<-stopCh
}
