# Cloudflare Dynamic DNS Kubernetes Controller

## Description

A small kubernetes controller that will create Cloudflare DNS records and watch for your dynamic IP address to change and update Cloudflare with the new IP.

## Deploy the controller

``` bash
kubectl create namespace cloudflare-dynamic-dns-controller
kubectl create secret -n cloudflare-dynamic-dns-controller generic cloudflare --from-literal=email=INSERT-EMAIL --from-literal=token=INSERT-TOKEN --from-literal=zone=INSERT-ZONE-ID
kubectl apply -n cloudflare-dynamic-dns-controller -f deploy.yml
```

## Creating a Cloudflare record

To use the controller add the annotations to either a server or an ingress resource.

``` yaml
---
apiVersion: v1
kind: Service
metadata:
  name: "example-website"
  annotations:
    cloudflare-dynamic-dns.alpha.kubernetes.io/hostname: "hello.example.com"
    cloudflare-dynamic-dns.alpha.kubernetes.io/proxied: "true"
spec:
  type: NodePort
  selector:
    app: example-website
  ports:
    - protocol: TCP
      port: 80
      targetPort: 80
```

## Annotations
Currently there are only 2 supported annotations:

#### Hostname
The name of the record. Note that this must be the full domain name including the zone. For example you must use `hello.example.com` instead of just `hello`

``` yaml
cloudflare-dynamic-dns.alpha.kubernetes.io/hostname: "hello.example.com"
```

#### Proxied
Whether to use the Cloudflare proxy to take advantage of all the benefits of Cloudflare. This is recommended becuase it will hide your public IP.

``` yaml
cloudflare-dynamic-dns.alpha.kubernetes.io/proxied: "true"
```