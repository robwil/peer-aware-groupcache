# peer-aware-groupcache

## Running locally

```
$ docker build -t peer-aware-groupcache .
$ docker run -p 5000:5000 -it peer-aware-groupcache
```

## Deployment

```
$ docker build -t peer-aware-groupcache .
$ docker tag peer-aware-groupcache robwil/peer-aware-groupcache:1.0.6
$ docker push robwil/peer-aware-groupcache:1.0.6
```

## Helm

```
$ helm install -n peer-aware-groupcache helm-chart/
```