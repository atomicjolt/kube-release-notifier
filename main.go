package main

import (
    "flag"
    "log"
    "os"
    "path/filepath"
    "time"
    "fmt"

    "k8s.io/client-go/informers"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
    "k8s.io/client-go/rest"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var kubeconfig string

func init() {
    flag.StringVar(&kubeconfig, "kubeconfig", filepath.Join(os.Getenv("HOME"), ".kube", "config"), "absolute path to the kubeconfig file")
}

func main() {
    // try to create an in-cluster config
    config, err := rest.InClusterConfig()
    if err != nil {
        // fall back to kubeconfig
        config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
        if err != nil {
            panic(err.Error())
        }
    }
    clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
        log.Fatal(err)
    }
    labelOptions := informers.WithTweakListOptions(func(options *metav1.ListOptions) {
        options.LabelSelector = "atomicjolt.com/release-notifier=enabled"
    })
    factory := informers.NewSharedInformerFactoryWithOptions(
        clientset,
        5*time.Minute,
        labelOptions,
    )

    controller, err := NewDeploymentMonitoringController(factory, *clientset)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("STARTING MONITORING\n")

    stop := make(chan struct{})
    defer close(stop)
    err = controller.Run(stop)
    if err != nil {
        log.Fatal(err)
    }
    select {}
}
