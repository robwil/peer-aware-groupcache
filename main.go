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
    "os"
    "sort"
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
}

func Factors(w http.ResponseWriter, r *http.Request) {
    nStr := r.FormValue("n")
    var b []byte
    if err := PrimeFactorsGroup.Get(nil, nStr, groupcache.AllocatingByteSliceSink(&b)); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    fmt.Fprintf(w, "%s\n", b)
}

func Stats(w http.ResponseWriter, _ *http.Request) {
    stats := PrimeFactorsGroup.CacheStats(groupcache.MainCache)
    fmt.Fprintln(w, "Bytes:    ", stats.Bytes)
    fmt.Fprintln(w, "Items:    ", stats.Items)
    fmt.Fprintln(w, "Gets:     ", stats.Gets)
    fmt.Fprintln(w, "Hits:     ", stats.Hits)
    fmt.Fprintln(w, "Evictions:", stats.Evictions)
    fmt.Fprintln(w, "Self URL: ", selfUrl)
    fmt.Fprintf(w, "Current pod set: [%d] %v\n", len(podSet), podSet)
}

type PodSet map[string]bool
func (podSet PodSet) Keys() []string {
    keys := make([]string, len(podSet))
    i := 0
    for key := range podSet {
        keys[i] = key
        i++
    }
    sort.Strings(keys)
    return keys
}
func (podSet PodSet) String() string {
    return fmt.Sprintf("%v", podSet.Keys())
}

func GetInitialPods(clientset *kubernetes.Clientset, listOptions metav1.ListOptions, myIp string) (PodSet, error) {
    pods, err := clientset.CoreV1().Pods("default").List(listOptions)
    if err != nil {
        return nil, err
    }
    podSet := make(PodSet)
    podSet[getPodUrl(myIp)] = true
    for _, pod := range pods.Items {
        podIp := pod.Status.PodIP
        if isPodReady(&pod) && podIp != myIp {
            podSet[getPodUrl(podIp)] = true
        }
    }
    return podSet, nil
}

func MonitorPodState(clientset *kubernetes.Clientset, listOptions metav1.ListOptions, pool *groupcache.HTTPPool, myIp string, initialPods PodSet) {
    // When a kube pod is ADDED or DELETED, it goes through several changes which issue MODIFIED events.
    // By watching these MODIFIED events for times when we see a given podIp associated with a Pod READY condition
    // set to true or false, we can keep track of all podUrls which are ready to receive connections. This stream of
    // events is used to maintain an always-updated list of peers for groupcache.

    podSet = initialPods
    log.Printf("Initial pod list = %v", podSet)

    // begin watch API call
    watchInterface, err := clientset.CoreV1().Pods("default").Watch(listOptions)
    if err != nil {
        log.Printf("WARNING: error watching pods: %v", err)
        return
    }
    ch := watchInterface.ResultChan()
    for event := range ch {
        pod, ok := event.Object.(*v1.Pod)
        if !ok {
            log.Printf("WARNING: got non-pod object from pod watching: %v", event.Object)
            continue
        }
        podName := pod.Name
        podIp := pod.Status.PodIP
        podReady := isPodReady(pod)

        // Debug info
        switch event.Type {
        case "ADDED":
            log.Printf("ADDED pod %s with ip %s. Ready = %v", podName, podIp, podReady)
        case "MODIFIED":
            log.Printf("MODIFIED pod %s with ip %s. Ready = %v", podName, podIp, podReady)
        case "DELETED":
            log.Printf("DELETED pod %s with ip %s. Ready = %v", podName, podIp, podReady)
        }

        if event.Type == "MODIFIED" && podIp != "" && podIp != myIp {
            podUrl := getPodUrl(podIp)
            if podReady && !podSet[podUrl] {
                // add IP to peer list
                log.Printf("Newly ready pod %s @ %s", podName, podUrl)
                podSet[podUrl] = true
            } else if !podReady && podSet[podUrl] {
                // remove IP from peer list
                log.Printf("Newly disappeared pod %s @ %s", podName, podUrl)
                delete(podSet, podUrl)
            } else {
                continue // no change to pod list
            }
            podUrls := podSet.Keys()
            log.Printf("New pod list = %v", podUrls)
            pool.Set(podUrls...)
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

func getPodUrl(podIp string) string {
    return fmt.Sprintf("http://%s:%d", podIp, Port)
}

func logRequest(handler http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
        handler.ServeHTTP(w, r)
    })
}

var selfUrl string
var podSet PodSet

func main() {
    // TODO: refactor this peer awareness as a library, and setup this groupcache app as a sample app for how to use it.
    // API could be something like:
    //    peerAwareness(metav1.ListOptions{}, updateFunc)
    // where updateFunc(podIp string, pod *v1.Pod, status PeerStatus) error - will be called as a goroutine whenever there is a change to the peer list
    // where PeerStatus = Added or Removed

    listOptions := metav1.ListOptions{LabelSelector: "app=peer-aware-groupcache"}

    // Setup Kube api connection, if within Kube cluster
    config, err := rest.InClusterConfig()
    var pool *groupcache.HTTPPool
    if err == nil {
        kubeClient, err := kubernetes.NewForConfig(config)
        if err != nil {
            log.Fatalf("Could not connect to Kubernetes API: %v", err)
        }
        myIp := os.Getenv("MY_POD_IP")
        if myIp == "" {
            log.Fatalf("Could not get self pod ip")
        }
        initialPods, err := GetInitialPods(kubeClient, listOptions, myIp)
        if err != nil {
            log.Fatalf("Could not get initial pod list: %v", err)
        }
        if len(initialPods) <= 0 {
            log.Fatalf("No pods detected, not even self!")
        }
        podUrls := initialPods.Keys()
        selfUrl = getPodUrl(myIp)
        pool = groupcache.NewHTTPPool(selfUrl)
        pool.Set(podUrls...)
        // Start monitoring for pod transitions, to keep pool up to date
        go MonitorPodState(kubeClient, listOptions, pool, myIp, initialPods)
    } else {
        log.Printf("WARNING: unable to create InCluster config for Kubernetes API. Is this running within a Kube cluster??")
        // Setup groupcache with just self as peer
        url := fmt.Sprintf("http://0.0.0.0:%d", Port)
        pool = groupcache.NewHTTPPool(url)
        pool.Set(url)
    }

    // Setup http routes
    http.HandleFunc("/", Index)
    http.HandleFunc("/factors", Factors)
    http.HandleFunc("/stats", Stats)

    log.Printf("Listening on port %d...", Port)
    if err := http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", Port), logRequest(http.DefaultServeMux)); err != nil {
        log.Fatalf("error in ListenAndServe: %s", err)
    }
}