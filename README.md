# echo-app

![Build and Push Docker Image](https://github.com/philipschmid/echo-app/actions/workflows/build.yaml/badge.svg) ![Go Syntax and Format Check](https://github.com/philipschmid/echo-app/actions/workflows/test.yaml/badge.svg)

Tiny golang app which returns a timestamp, a customizable message, the hostname, the request source IP, the node name (if set), and optionally the HTTP request headers.

## Configuration Options
- `MESSAGE`: A customizable message to be returned in the JSON response. If not set, no message will be displayed.
- `NODE`: The name of the node where the app is running. This is typically used in a Kubernetes environment.
- `PORT`: The port number on which the server listens. Default is `8080`.
- `PRINT_HTTP_REQUEST_HEADERS`: Set to `true` to include HTTP request headers in the JSON response. By default, headers are not included.
- `TLS`: Set to `true` to enable TLS (HTTPS) support. By default, TLS is disabled.
- `TLS_PORT`: The port number on which the TLS server listens. Default is `8443`.

## Standalone Container
Shell 1 (server):
```bash
docker run -it -p 8080:8080 ghcr.io/philipschmid/echo-app:main
# Optionally with a customized message:
docker run -it -p 8080:8080 -e MESSAGE="demo-env" ghcr.io/philipschmid/echo-app:main
# Optionally with a node name:
docker run -it -p 8080:8080 -e NODE="k8s-node-1" ghcr.io/philipschmid/echo-app:main
# Optionally include HTTP request headers in the response:
docker run -it -p 8080:8080 -e PRINT_HTTP_REQUEST_HEADERS="true" ghcr.io/philipschmid/echo-app:main
# Optionally enable TLS:
docker run -it -p 8080:8080 -p 8443:8443 -e TLS="true" ghcr.io/philipschmid/echo-app:main
```

Shell 2 (client):
```bash
curl http://localhost:8080/
```

You should see a similar client output like this:
```json
{"timestamp":"2024-05-28T19:50:10.289Z","hostname":"83ff0b127ed6","source_ip":"192.168.65.1"}
{"timestamp":"2024-05-28T19:50:35.022Z","message":"Hello World!","hostname":"4495529ebd32","source_ip":"192.168.65.1","node":"k8s-node-1"}
```

If `PRINT_HTTP_REQUEST_HEADERS` is set to `true`, the response will also include the request headers:
```json
{"timestamp":"2024-05-28T20:21:23.363Z","hostname":"3f96391b04f2","source_ip":"192.168.65.1","node":"k8s-node-1","headers":{"Accept":["*/*"],"User-Agent":["curl/8.6.0"]}}
```

If `TLS` is enabled, you can test the HTTPS endpoint:
```bash
curl -k https://localhost:8443/
```

## Kubernetes
Apply the following manifests to deploy the echo-app with the `NODE` environment variable set to the name of the Kubernetes node using the Downward API:
```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: echo-app-config
data:
  MESSAGE: "demo-env"
  # Add the PRINT_HTTP_REQUEST_HEADERS key with a value of "true" to include headers in the response
  PRINT_HTTP_REQUEST_HEADERS: "true"
  # Add the TLS key with a value of "true" to enable TLS
  TLS: "true"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echo-app-deployment
  labels:
    app: echo-app
spec:
  replicas: 2  # Adjust the number of replicas as needed
  selector:
    matchLabels:
      app: echo-app
  template:
    metadata:
      labels:
        app: echo-app
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            - labelSelector:
                matchExpressions:
                  - key: "app"
                    operator: In
                    values:
                    - echo-app
              topologyKey: "kubernetes.io/hostname"
      containers:
      - name: echo-app
        image: ghcr.io/philipschmid/echo-app:main
        ports:
        - containerPort: 8080
        - containerPort: 8443
        env:
        - name: MESSAGE
          valueFrom:
            configMapKeyRef:
              name: echo-app-config
              key: MESSAGE
        # Add the PRINT_HTTP_REQUEST_HEADERS environment variable
        - name: PRINT_HTTP_REQUEST_HEADERS
          valueFrom:
            configMapKeyRef:
              name: echo-app-config
              key: PRINT_HTTP_REQUEST_HEADERS
        # Add the TLS environment variable
        - name: TLS
          valueFrom:
            configMapKeyRef:
              name: echo-app-config
              key: TLS
        # Add the NODE environment variable using the downward API
        - name: NODE
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
---
apiVersion: v1
kind: Service
metadata:
  name: echo-app-service
spec:
  selector:
    app: echo-app
  ports:
    - protocol: TCP
      port: 8080
      targetPort: 8080
    - protocol: TCP
      port: 8443
      targetPort: 8443
  type: ClusterIP
```

Shell 2 (client):
```bash
kubectl run netshoot --rm -it --image=nicolaka/netshoot -- curl http://echo-app-service:8080
```

You should see a similar client output like this:
```json
{"timestamp":"2024-05-28T19:50:35.022Z","message":"demo-env","hostname":"echo-app-deployment-5d8f8b8b8b-9t4kq","source_ip":"10.1.0.1","node":"k8s-node-1"}
```

If `PRINT_HTTP_REQUEST_HEADERS` is set to `true`, the response will also include the request headers:
```json
{"timestamp":"2024-05-28T20:21:23.363Z","message":"demo-env","hostname":"echo-app-deployment-5d8f8b8b8b-9t4kq","source_ip":"10.1.0.1","node":"k8s-node-1","headers":{"Accept":["*/*"],"User-Agent":["curl/8.6.0"]}}
```

## Credit
Basic idea (& source code) is taken from https://cloud.google.com/kubernetes-engine/docs/samples/container-hello-app.