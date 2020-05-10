# Cloudflare Dynamic DNS Kubernetes Controller

``` bash
kubectl create namespace cloudflare-dynamic-dns-controller
kubectl create secret -n cloudflare-dynamic-dns-controller generic cloudflare --from-literal=email=INSERT-EMAIL --from-literal=token=INSERT-TOKEN --from-literal=zone=INSERT-ZONE-ID
kubectl apply -n cloudflare-dynamic-dns-controller -f deploy.yml
```
