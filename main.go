package main

import (
    "fmt"
    "log"
    "github.com/valyala/fasthttp"
    "github.com/qiangxue/fasthttp-routing"
    "strconv"
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

func Index(ctx *routing.Context) error {
    log.Printf("GET /")
    fmt.Fprintf(ctx, "Hello world\n")
    return nil
}

func Factors(ctx *routing.Context) error {
    log.Printf("GET %s", ctx.RequestURI())
    nStr := ctx.Param("n")
    n, err := strconv.ParseInt(nStr, 10, 64)
    if err != nil {
        return err
    }
    pfs := PrimeFactors(n)
    fmt.Fprintf(ctx, "%v\n", pfs)
    return nil
}

func main() {
    // Setup http routes
    router := routing.New()
    router.Get("/", Index)
    router.Get("/factors/<n>", Factors)

    log.Printf("Listening on port %d...", Port)
    if err := fasthttp.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", Port), router.HandleRequest); err != nil {
        log.Fatalf("error in ListenAndServe: %s", err)
    }
}