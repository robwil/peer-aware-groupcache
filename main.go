package main

import (
    "fmt"
    "log"
    "github.com/valyala/fasthttp"
    "github.com/qiangxue/fasthttp-routing"
)

const Port = 5000

func Index(ctx *routing.Context) error {
    log.Printf("GET /")
    fmt.Fprintf(ctx, "Hello world")
    return nil
}

func main() {
    // Setup http routes
    router := routing.New()
    router.Get("/", Index)

    log.Printf("Listening on port %d...", Port)
    if err := fasthttp.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", Port), router.HandleRequest); err != nil {
        log.Fatalf("error in ListenAndServe: %s", err)
    }
}