package main

import (
    "fmt"
    "log"
    "strconv"
    "github.com/golang/groupcache"
    "net/http"
    "k8s.io/client-go/kubernetes"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/api/core/v1"
    "k8s.io/client-go/rest"
)

const Port = 5000

// Get all prime factors of a given number n
// Reference: https://siongui.github.io/2017/05/09/go-find-all-prime-factors-of-integer-number/
func PrimeFactors(n int64) (pfs []int64) {
    // Get the number of 2s that divide n
    for n%2 == 0 {
        pfs = append(pfs, 2)
        n = n / 2
    }

    // n must be odd at this point. so we can skip one element
    // (note i = i + 2)
    for i := int64(3); i*i <= n; i = i + 2 {
        // while i divides n, append i and divide n
        for n%i == 0 {
            pfs = append(pfs, i)
            n = n / i
        }
    }

    // This condition is to handle the case when n is a prime number
    // greater than 2
    if n > 2 {
        pfs = append(pfs, n)
    }

    return
}

var PrimeFactorsGroup = groupcache.NewGroup("primeFactors", 1 << 20, groupcache.GetterFunc(func(ctx groupcache.Context, key string, dest groupcache.Sink) error {
    log.Printf("Calculating prime factors for %s", key)
    n, err := strconv.ParseInt(key, 10, 64)
    if err != nil {
        return err
    }
    pfs := PrimeFactors(n)
    dest.SetString(fmt.Sprintf("%v", pfs))
    return nil
}))

func Index(w http.ResponseWriter, _ *http.Request) {
    fmt.Fprintf(w, "Hello world\n")
    log.Printf("GET /")
}

func Factors(w http.ResponseWriter, r *http.Request) {
    nStr := r.FormValue("n")
    var b []byte
    if err := PrimeFactorsGroup.Get(nil, nStr, groupcache.AllocatingByteSliceSink(&b)); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    fmt.Fprintf(w, "%s\n", b)
    log.Printf("GET %s", r.RequestURI)
}

func MonitorPodState(clientset *kubernetes.Clientset, pool *groupcache.HTTPPool) {
    // When a kube pod is ADDED or DELETED, it goes through several changes which issue MODIFIED events.
    // By watching these MODIFIED events for times when we see a given podIp associated with a Pod READY condition
    // set to true or false, we can keep track of all pods which are ready to receive connections. This stream of
    // events is used to maintain an always-updated list of peers for groupcache.

    // TODO: add some filtering here to only peer-aware-groupcache pods
    watchInterface, err := clientset.CoreV1().Pods("default").Watch(metav1.ListOptions{})
    if err != nil {
        log.Printf("WARNING: error watching pods: %v", err)
        return
    }
    ch := watchInterface.ResultChan()
    log.Printf("Started waiting on result channel...")
    for event := range ch {

        pod, ok := event.Object.(*v1.Pod)
        if !ok {
            log.Printf("WARNING: got non-pod object from pod watching: %v", event.Object)
        }
        // TODO: listen to all events where podIp is present
        // If podReady = false, remove that IP from peer list
        // If podReady = true, add that IP to peer list
        podName := pod.Name
        podIp := pod.Status.PodIP
        podReady := false
        for _, condition := range pod.Status.Conditions {
            if condition.Type == v1.PodReady && condition.Status == v1.ConditionTrue {
                podReady = true
            }
        }
        switch event.Type {
        case "ADDED":
            log.Printf("ADDED pod %s with ip %s. Ready = %v", podName, podIp, podReady)
        case "MODIFIED":
            log.Printf("MODIFIED pod %s with ip %s. Ready = %v", podName, podIp, podReady)
        case "DELETED":
            log.Printf("DELETED pod %s with ip %s. Ready = %v", podName, podIp, podReady)
        }
    }
}

func main() {
    // TODO: refactor this peer awareness as a library, and setup this groupcache app as a sample app for how to use it.
    // API could be something like:
    //    peerAwareness(filterFunc, updateFunc)
    // where filterFunc(*v1.Pod) bool - defines how to filter pod events
    // where updateFunc(podIp string, pod *v1.Pod, status PeerStatus) error - will be called as a goroutine whenever there is a change to the peer list
    // where PeerStatus = Added or Removed

    // Setup groupcache (will automatically inject handler into net/http)
    me := fmt.Sprintf("0.0.0.0:%d", Port)

    // TODO: kube api call to get initial list of peers.
    pool := groupcache.NewHTTPPool("http://" + me)
    pool.Set(me)

    // Setup Kube api connection, if within Kube cluster
    config, err := rest.InClusterConfig()
    if err == nil {
        clientset, err := kubernetes.NewForConfig(config)
        if err != nil {
            log.Fatalf("Could not connect to Kubernetes API: %v", err)
        }
        // Start monitoring for pod transitions
        go MonitorPodState(clientset, pool)
    } else {
        log.Printf("WARNING: unable to create InCluster config for Kubernetes API. Is this running within a Kube cluster??")
    }

    // Setup http routes
    http.HandleFunc("/", Index)
    http.HandleFunc("/factors", Factors)

    log.Printf("Listening on port %d...", Port)
    if err := http.ListenAndServe(me, nil); err != nil {
        log.Fatalf("error in ListenAndServe: %s", err)
    }
}