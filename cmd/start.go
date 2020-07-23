/*
Copyright © 2020 Aaron France <aaron.l.france@gmail.com>

*/
package cmd

import (
	"flag"
	"os"

	"github.com/AeroNotix/k8s-event/pkg/alerting"
	"github.com/AeroNotix/k8s-event/pkg/listening"
	"github.com/AeroNotix/k8s-event/pkg/listening/healthchecks"
	"github.com/AeroNotix/k8s-event/pkg/listening/oomkill"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

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

type Start struct {
}

func (s Start) Run() {
	clientset := loadConfig()
	alerterRegistry := alerting.NewRegistryFromConfig(viper.GetViper())
	sharedInformerFactory := informers.NewSharedInformerFactory(clientset, viper.GetDuration("resync-interval"))
	stop := listening.SigHandler()
	wg := listening.RunAll([]listening.Listener{
		oomkill.New(clientset, sharedInformerFactory, alerterRegistry.GetAlerter("oomkill")),
		healthchecks.New(clientset, sharedInformerFactory, alerterRegistry.GetAlerter("oomkill")),
	})
	glog.Infof("Starting shared Informer(s)")
	sharedInformerFactory.Start(stop)
	wg.Wait()
	glog.Warningf("Exiting main()")
	os.Exit(1)
}

func init() {
	s := Start{}
	var startCmd = &cobra.Command{
		Use:   "start",
		Short: "Starts the application",
		Run: func(cmd *cobra.Command, args []string) {
			s.Run()
		},
	}
	rootCmd.AddCommand(startCmd)
}
