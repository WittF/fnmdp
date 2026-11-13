# deploy

Example deployment for fnmdp. Everything here is a **sample**: the namespace
(`fnmdp`), domain (`fnmdp.example.com`), Gateway name (`example-gateway`) and
images (`ghcr.io/wittf/fnmdp-*`, no longer updated) are placeholders — replace
them with your own. No credentials or endpoints are committed; create them
out-of-band as shown below.

## Layout
- `k8s/` — manifests for a `fnmdp` namespace (master + web + edge route).
  - `configmap.yaml` — non-sensitive master runtime defaults (scheduler intervals, UA).
  - `secret.example.yaml` — template for `fnmdp-secrets` (real values created out-of-band).
  - `master.yaml` — master Deployment (single replica, `/healthz` probes) + ClusterIP Service. Reads endpoints from the out-of-band `fnmdp-master-conn` ConfigMap.
  - `web.yaml` — nginx static SPA Deployment + Service.
  - `httproute.yaml` — Gateway API `HTTPRoute`; `/api`,`/live`,`/ingest` → master, `/` → web.
- `nodes/` — collector fleet.
  - `docker-compose.yml` — `fnmdp-node` + watchtower (self-update on `:latest`).
  - `deploy-nodes.sh` — batch SSH installer; reads a `hosts.txt` (IP + node-id) that you keep out of the repo.

Postgres and Valkey are optional: without them the master falls back to
in-memory storage / an in-process dedup window (fine for trying it out, not for
keeping history).

## Bootstrap (one-time, values never committed)

```sh
kubectl create namespace fnmdp

# 1. Connection ConfigMap (endpoints — not in git):
kubectl -n fnmdp create configmap fnmdp-master-conn \
  --from-literal=PGHOST='<postgres host>' \
  --from-literal=PGPORT='5432' \
  --from-literal=PGUSER='<postgres user>' \
  --from-literal=PGDATABASE='postgres' \
  --from-literal=PGSSLMODE='require' \
  --from-literal=VALKEY_ADDR='<valkey host:port>' \
  --from-literal=VALKEY_DB='0' \
  --from-literal=VALKEY_TLS='true'

# 2. Secret (passwords / tokens — not in git):
kubectl -n fnmdp create secret generic fnmdp-secrets \
  --from-literal=pg-password='<postgres password>' \
  --from-literal=valkey-password='<valkey password>' \
  --from-literal=node-token="$(openssl rand -hex 32)" \
  --from-literal=admin-token="$(openssl rand -hex 32)"

# 3. Apply manifests (edit hostname / Gateway in httproute.yaml first):
kubectl -n fnmdp apply -f deploy/k8s/configmap.yaml \
  -f deploy/k8s/master.yaml -f deploy/k8s/web.yaml -f deploy/k8s/httproute.yaml

# 4. Verify:
kubectl -n fnmdp rollout status deploy/fnmdp-master deploy/fnmdp-web
curl https://fnmdp.example.com/api/snapshot
```

> **Always set `node-token` and `admin-token`.** If they are empty the master
> accepts any node and any admin request.

## Continuous deploy
Push to `main` → CI builds and tests. With the `PUBLISH_IMAGES` repo variable
set to `true` it also pushes `:latest` and `:<sha>`; with `DEPLOY_ENABLED` too
(plus the `KUBE_CONFIG` / `K8S_NAMESPACE` secrets) it rolls master/web to the
pinned `:<sha>` with a least-privilege service account (deployments
get/list/watch/patch only). Publishing and deploying only run in the upstream
repository; forks build and test.

## Nodes
Point `docker-compose.yml` at your own node image. If your registry is
private, add a `docker login` step and give watchtower the credentials.

```sh
MASTER_URL=wss://fnmdp.example.com/ingest NODE_TOKEN=<same as secret> SSH_PASS=<ssh password> \
  ./deploy/nodes/deploy-nodes.sh hosts.txt 3   # first 3 only; drop the 3 for all
```
