package main

import (
    "fmt"
    "log"
    "strconv"
    "github.com/golang/groupcache"
    "net/http"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "github.com/robwil/peer-aware-groupcache/peerwatch"
    "sort"
    "os"
    "time"
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
    fmt.Fprintf(w, "Current pod set: [%d] %v\n", len(urlSet), urlSet)
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

type UrlSet map[string]bool
func (urlSet UrlSet) Keys() []string {
    keys := make([]string, len(urlSet))
    i := 0
    for key := range urlSet {
        keys[i] = key
        i++
    }
    sort.Strings(keys)
    return keys
}
func (urlSet UrlSet) String() string {
    return fmt.Sprintf("%v", urlSet.Keys())
}

var selfUrl string
var urlSet UrlSet

const DebugMode = true

func main() {
    urlSet = make(UrlSet)
    myIp := os.Getenv("MY_POD_IP")
    listOptions := metav1.ListOptions{LabelSelector: "app=peer-aware-groupcache"}

    var pool *groupcache.HTTPPool
    initialized := false
    initialPeers, err := peerwatch.Init(myIp, listOptions, func(ip string, state peerwatch.NotifyState) {
        for !initialized {
            // prevent race condition by waiting for initial peers to be setup before processing any changes
            time.Sleep(100 * time.Millisecond)
        }
        if DebugMode {
            log.Printf("Got notify: %s [%d]", ip, state)
        }
        switch state {
        case peerwatch.Added:
            urlSet[getPodUrl(ip)] = true
        case peerwatch.Removed:
            delete(urlSet, getPodUrl(ip))
        default:
            return
        }
        podUrls := urlSet.Keys()
        log.Printf("New pod list = %v", podUrls)
        pool.Set(podUrls...)
    }, DebugMode)
    if err != nil {
        // Setup groupcache with just self as peer
        log.Printf("WARNING: error getting initial pods: %v", err)
        url := fmt.Sprintf("http://0.0.0.0:%d", Port)
        urlSet[url] = true
        pool = groupcache.NewHTTPPool(url)
        pool.Set(url)
        initialized = true
    } else {
        for _, ip := range initialPeers {
            urlSet[getPodUrl(ip)] = true
        }
        selfUrl = getPodUrl(myIp)
        podUrls := urlSet.Keys()
        pool = groupcache.NewHTTPPool(selfUrl)
        pool.Set(podUrls...)
        initialized = true
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