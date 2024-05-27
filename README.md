# echo-app

![Build and Push Docker Image](https://github.com/philipschmid/echo-app/actions/workflows/build.yaml/badge.svg)

Tiny golang app which returns a customizable message, the hostname, and the request source IP.

## Standalone Container
Shell 1 (server):
```bash
docker run -it -p 8080:8080 ghcr.io/philipschmid/echo-app:main
# Or optionally with a customized message:
# docker run -it -p 8080:8080 -e MESSAGE="demo-env" ghcr.io/philipschmid/echo-app:main
```

Shell 2 (client):
```bash
curl http://localhost:8080/
```

You should see a similar client output like this:
```json
{"message":"Hello, world!","hostname":"e4442ea9e53c","source_ip":"192.168.65.1"}
{"message":"demo-env","hostname":"f4c4b96e362d","source_ip":"192.168.65.1"}
```

## Kubernetes
Apply the following manifests:
```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: echo-app-config
data:
  MESSAGE: "demo-env"
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
        env:
        - name: MESSAGE
          valueFrom:
            configMapKeyRef:
              name: echo-app-config
              key: MESSAGE
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
  type: ClusterIP
```

Shell 2 (client):
```bash
kubectl run netshoot --rm -it --image=nicolaka/netshoot -- curl http://echo-app-service:8080
```

## Credit
Basic idea (& source code) is taken from https://cloud.google.com/kubernetes-engine/docs/samples/container-hello-app.