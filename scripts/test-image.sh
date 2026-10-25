#!/usr/bin/env bash
set -euo pipefail

image=${1:?usage: test-image.sh IMAGE}
container=$(docker run -d --read-only --cap-drop=ALL --security-opt=no-new-privileges \
  -p 127.0.0.1::8080 -p 127.0.0.1::3000 "$image" --message=image-smoke)
trap 'docker logs "$container"; docker rm -f "$container" >/dev/null' EXIT
http_port=$(docker port "$container" 8080/tcp | cut -d: -f2)
admin_port=$(docker port "$container" 3000/tcp | cut -d: -f2)

for attempt in {1..50}; do
  if curl -fsS --max-time 1 "http://127.0.0.1:$admin_port/ready" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done
test "$(curl -fsS --max-time 3 "http://127.0.0.1:$admin_port/ready")" = ready
curl -fsS --max-time 3 "http://127.0.0.1:$http_port/" | jq -e '.listener == "HTTP" and .message == "image-smoke"'
test "$(curl -fsS --max-time 3 "http://127.0.0.1:$admin_port/health")" = healthy
curl -fsS --max-time 3 "http://127.0.0.1:$admin_port/metrics" | grep echo_observatory_requests_total
test "$(docker inspect --format '{{.Config.User}}' "$container")" = 65532:65532
docker stop --timeout 10 "$container" >/dev/null
test "$(docker inspect --format '{{.State.ExitCode}}' "$container")" = 0
