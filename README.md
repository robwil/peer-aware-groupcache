# peer-aware-groupcache

This is a proof of concept, showing how the Kubernetes `watch` API can be used to keep an up-to-date list of all peer pods.

The repo has two parts:
* the `peerwatch` package, which is a library that knows how to monitor Kubernetes for pod changes
* the `peer-aware-groupcache` example application, which is a toy HTTP API computing prime factors, caching the results
with [groupcache](https://github.com/golang/groupcache). It uses the `peerwatch` package to ensure any added or removed
pods are also removed from `groupcache`'s peer list.

TODO: document how to use peerwatch library. and/or generate `godoc` for it

## Running

```
$ helm install -n peer-aware-groupcache helm-chart/
$ kubectl scale --replicas=3 deployment/peer-aware-groupcache
$ export NODE_PORT=$(kubectl get --namespace default -o jsonpath="{.spec.ports[0].nodePort}" services peer-aware-groupcache)
$ export NODE_IP=$(kubectl get nodes --namespace default -o jsonpath="{.items[0].status.addresses[0].address}")
$ for i in `seq 0 9`; do echo 1234$i; curl http://$NODE_IP:$NODE_PORT/factors?n=1234$i; done
```

## Development

Notes to self about how to publish new versions of this.

### Building image

```
$ docker build -t peer-aware-groupcache .
$ docker tag peer-aware-groupcache robwil/peer-aware-groupcache:1.2.2
$ docker push robwil/peer-aware-groupcache:1.2.2
```

### Running locally

Note that running locally doesn't do anything with Kube peer awareness, since it isn't inside a Kube cluster.

```
$ docker build -t peer-aware-groupcache .
$ docker run -p 5000:5000 -it peer-aware-groupcache
```



