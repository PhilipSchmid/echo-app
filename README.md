# echo-app
Tiny golang app which returns a customizable message, the hostname and the request source IP

# Usage
Shell 1 (server):
```bash
docker run -it -p 8080:8080 ghcr.io/philipschmid/echo-app:latest
# Or optionally with a customized message:
# docker run -it -p 8080:8080 -e MESSAGE="demo-env" ghcr.io/philipschmid/echo-app:latest
```

Shell 2 (client):
```bash
curl http://localhost:8080/
```

You should see a similar client output like this:
```bash
TBD
```

# Development
## Manual Build
```bash
docker buildx build --platform linux/amd64,linux/arm64 -t pschmid/echo-app:latest --push .
```

# Credit
Source code is based on https://cloud.google.com/kubernetes-engine/docs/samples/container-hello-app.