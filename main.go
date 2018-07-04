package main

import (
    "fmt"
    "log"
    "strconv"
    "github.com/golang/groupcache"
    "net/http"
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

func main() {
    // Setup groupcache (will automatically inject handler into net/http)
    me := fmt.Sprintf("0.0.0.0:%d", Port)
    pool := groupcache.NewHTTPPool("http://" + me)
    pool.Set(me)
    // Whenever peers change:
    //peers.Set("http://10.0.0.1", "http://10.0.0.2", "http://10.0.0.3")

    http.HandleFunc("/", Index)
    http.HandleFunc("/factors", Factors)

    log.Printf("Listening on port %d...", Port)
    if err := http.ListenAndServe(me, nil); err != nil {
        log.Fatalf("error in ListenAndServe: %s", err)
    }
}