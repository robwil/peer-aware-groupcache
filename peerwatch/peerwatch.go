package peerwatch

import (
    "k8s.io/client-go/kubernetes"
    "log"
    "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/rest"
    "fmt"
    "errors"
)

type config struct {
    debugMode bool
}

var libConfig config

func debugLogf(format string, v ...interface{}) {
    if libConfig.debugMode {
        log.Printf(format, v...)
    }
}

func getInitialPods(clientset *kubernetes.Clientset, listOptions metav1.ListOptions, myIp string) (podSet, error) {
    pods, err := clientset.CoreV1().Pods("default").List(listOptions)
    if err != nil {
        return nil, err
    }
    podSet := make(podSet)
    podSet[myIp] = true
    for _, pod := range pods.Items {
        podIp := pod.Status.PodIP
        if isPodReady(&pod) && podIp != myIp {
            podSet[podIp] = true
        }
    }
    return podSet, nil
}

func monitorPodState(clientset *kubernetes.Clientset, listOptions metav1.ListOptions, myIp string, initialPods podSet, f NotifyFunc) {
    // When a kube pod is ADDED or DELETED, it goes through several changes which issue MODIFIED events.
    // By watching these MODIFIED events for times when we see a given podIp associated with a Pod READY condition
    // set to true or false, we can keep track of all pod ip addresses which are ready to receive connections.

    podSet := initialPods
    debugLogf("Initial pod list = %v", podSet)

    // begin watch API call
    watchInterface, err := clientset.CoreV1().Pods("default").Watch(listOptions)
    if err != nil {
        debugLogf("WARNING: error watching pods: %v", err)
        return
    }

    // React to watch result channel
    ch := watchInterface.ResultChan()
    for event := range ch {
        pod, ok := event.Object.(*v1.Pod)
        if !ok {
            debugLogf("WARNING: got non-pod object from pod watching: %v", event.Object)
            continue
        }

        podName := pod.Name
        podIp := pod.Status.PodIP
        podReady := isPodReady(pod)

        // Log raw event stream to debug log
        switch event.Type {
        case "ADDED":
            debugLogf("ADDED pod %s with ip %s. Ready = %v", podName, podIp, podReady)
        case "MODIFIED":
            debugLogf("MODIFIED pod %s with ip %s. Ready = %v", podName, podIp, podReady)
        case "DELETED":
            debugLogf("DELETED pod %s with ip %s. Ready = %v", podName, podIp, podReady)
        }

        // Main events we care about: MODIFIED including a PodIp other than current pod's IP
        if event.Type == "MODIFIED" && podIp != "" && podIp != myIp {
            if podReady && !podSet[podIp] {
                debugLogf("Newly ready pod %s @ %s", podName, podIp)
                podSet[podIp] = true
                go f(podIp, Added)
            } else if !podReady && podSet[podIp] {
                debugLogf("Newly disappeared pod %s @ %s", podName, podIp)
                delete(podSet, podIp)
                go f(podIp, Removed)
            } else {
                continue // no change to pod list
            }
        }
    }
}

func isPodReady(pod *v1.Pod) bool {
    for _, condition := range pod.Status.Conditions {
        if condition.Type == v1.PodReady && condition.Status == v1.ConditionTrue {
            return true
        }
    }
    return false
}

type NotifyState int
const (
    Added   NotifyState = 1
    Removed NotifyState = 2
)

type NotifyFunc func(ip string, state NotifyState)

// Init initializes the peerwatch library, returning the initial set of pod ips
// and then continually monitors for changes, notifying notifyFunc whenever a pod
// change occurs.
//
// myIp is the IP of the current pod
// listOptions will be used in the calls to Kubernetes API, to filter to desired pods (e.g. by LabelSelector)
// f is a NotifyFunc that lets you do whatever you want with the incoming pod change events. Note this will be called in goroutines so should include thread-safe logic.
// debugMode controls whether to log debug messages or not
func Init(myIp string, listOptions metav1.ListOptions, f NotifyFunc, debugMode bool) ([]string, error) {

    libConfig.debugMode = debugMode

    // Setup Kube api connection, using the in-cluster config. This assumes the app is running in a pod.
    config, err := rest.InClusterConfig()
    if err != nil {
        return nil, err
    }
    kubeClient, err := kubernetes.NewForConfig(config)
    if err != nil {
        return nil, err
    }

    // Fetch initial pods from API
    initialPods, err := getInitialPods(kubeClient, listOptions, myIp)
    if err != nil {
        return nil, fmt.Errorf("could not get initial pod list: %v", err)
    }
    if len(initialPods) <= 0 {
        return nil, errors.New("no pods detected, not even self")
    }
    podIps := initialPods.Keys()

    // Start monitoring for pod transitions, to keep pool up to date
    go monitorPodState(kubeClient, listOptions, myIp, initialPods, f)

    return podIps, nil
}