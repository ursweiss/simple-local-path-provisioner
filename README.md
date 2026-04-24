# simple-local-path-provisioner

A minimal CSI driver for **K3s/k3d lab environments** that dynamically provisions
directory-backed persistent volumes with **deterministic, reproducible host-path names**.

Unlike Rancher's `local-path-provisioner`, which generates random per-volume directory
names using the PV UID, this driver derives the backing directory name solely from the
PVC namespace and name. The same PVC identity always maps to the same directory — even
after the cluster is deleted and recreated.

> **Lab use only.** This driver is intentionally simple and is not production-grade
> storage. It provides no encryption, no capacity enforcement, no snapshots, and no
> multi-host distribution.

---

## How It Works

For every PVC bound to the `simple-local-path` StorageClass, the driver:

1. Derives a backing directory path deterministically:
   ```
   {basePath}/{namespace}-{pvcName}
   ```
   Example: PVC `default/data-db-0` → `/srv/k3d-persistent-volumes/default-data-db-0`

2. Creates the directory if it does not exist, or reuses it silently if it does.

3. Tracks which node currently holds the writable publication in a hidden metadata file
   (`.csi-meta.json`) inside the backing directory.

4. Bind-mounts the backing directory into the pod's target path on the node.

When a PVC is deleted, **the backing directory is retained by default**. If the same PVC
name is recreated later (including after full cluster recreation), the same directory —
and all its data — is reused automatically.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Deployment (controller)                                    │
│  ┌──────────────┐  ┌────────────────────┐  ┌────────────┐  │
│  │ driver       │  │ external-provisioner│  │ external-  │  │
│  │ --mode=ctrl  │  │                    │  │ attacher   │  │
│  └──────────────┘  └────────────────────┘  └────────────┘  │
└─────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────┐
│  DaemonSet (node, one per cluster node)      │
│  ┌──────────────┐  ┌──────────────────────┐  │
│  │ driver       │  │ node-driver-registrar │  │
│  │ --mode=node  │  │                      │  │
│  └──────────────┘  └──────────────────────┘  │
└──────────────────────────────────────────────┘
```

- **Controller** — handles `CreateVolume`, `DeleteVolume`, `ControllerPublishVolume`,
  `ControllerUnpublishVolume`. Manages exclusive write-ownership tracking per volume.
- **Node** — handles `NodePublishVolume` (bind-mount) and `NodeUnpublishVolume` (unmount).
- **Standard K8s CSI sidecars** — `external-provisioner`, `external-attacher`, and
  `node-driver-registrar` handle the Kubernetes-facing lifecycle; the driver only
  implements the storage-facing CSI RPCs.

---

## Important: Naming and Collision Behaviour

The backing path is derived as:

```
sanitize(namespace) + "-" + sanitize(pvcName)
```

where `sanitize` lowercases the string and replaces any character outside `[a-z0-9.-]`
with a hyphen.

**This means two distinct PVC identities can collide if their sanitized forms are
identical.** For example:

| Namespace | PVC name | Backing directory      |
|-----------|----------|------------------------|
| `a-b`     | `c`      | `{base}/a-b-c`         |
| `a`       | `b-c`    | `{base}/a-b-c` ← same! |

**Mitigation:**

- In practice, Kubernetes namespace and PVC naming conventions make such collisions
  extremely unlikely in real workloads.
- StatefulSet PVCs (`{claim}-{name}-{ordinal}`) and their namespaces are typically
  well-separated.
- If you operate in an environment where collisions are possible, ensure your naming
  conventions are unambiguous.
- A hash-suffix option is intentionally **not implemented** — pure deterministic naming
  without any opaque suffix is the core feature of this driver. Inspect the backing
  directory name directly; it must always be human-readable and predictable.

---

## Requirements

- Kubernetes 1.20+ (K3s or k3d recommended)
- All cluster nodes must have the base path mounted at the **same absolute path**
- `attachRequired: true` is used — your cluster must support `VolumeAttachment` objects
  (all standard Kubernetes distributions do)
- The node containers run **privileged** (required for bind-mounts)

---

## Building

### Binary

```bash
make build
# output: bin/driver
```

### Docker image

```bash
make docker-build
# default tag: ghcr.io/ursweiss/simple-local-path-provisioner:latest

# custom tag:
make docker-build IMAGE_REPO=my.registry/simple-local-path-provisioner IMAGE_TAG=v0.1.0
```

The Dockerfile uses a multi-stage build (`golang:1.22-alpine` builder →
`distroless/static-debian12` runtime). The final image contains only the statically
linked driver binary — no shell, no package manager.

---

## Installation

### 1. Prepare the base directory on the host

All K3d/K3s nodes must have the base path available at the same absolute path. For k3d,
mount the host directory into every node at container creation:

```bash
k3d cluster create my-cluster \
  --volume /srv/k3d-persistent-volumes:/srv/k3d-persistent-volumes@all
```

### 2. Install with Helm

```bash
helm install simple-local-path \
  deploy/helm/simple-local-path-provisioner \
  --namespace simple-local-path \
  --create-namespace
```

To install with custom values:

```bash
helm install simple-local-path \
  deploy/helm/simple-local-path-provisioner \
  --namespace simple-local-path \
  --create-namespace \
  --set basePath=/srv/k3d-persistent-volumes \
  --set storageClass.isDefault=true
```

### 3. Verify installation

```bash
kubectl -n simple-local-path get pods
# expect: controller pod (1/1) and node pod per node (2/2)

kubectl get storageclass simple-local-path
```

---

## Configuration Reference

All values can be set via `--set` or a custom `values.yaml` file.

| Value | Default | Description |
|---|---|---|
| `image.repository` | `ghcr.io/ursweiss/simple-local-path-provisioner` | Driver image repository |
| `image.tag` | `0.1.0` | Driver image tag |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy |
| `driver.name` | `simple-local-path.csi.whity.ch` | CSI driver name (must be unique per cluster) |
| `basePath` | `/srv/k3d-persistent-volumes` | Host base directory for all backing directories |
| `staleTimeout` | `1m` | How long before a stale publish lock may be reclaimed (set `0` to disable) |
| `allowForceTakeover` | `false` | If `true`, any publish request overwrites the existing lock immediately |
| `storageClass.name` | `simple-local-path` | Name of the StorageClass to create |
| `storageClass.isDefault` | `false` | Set `true` to make this the cluster default StorageClass |
| `storageClass.reclaimPolicy` | `Retain` | PV reclaim policy (`Retain` or `Delete`; `Retain` is strongly recommended) |
| `storageClass.bindingMode` | `Immediate` | Volume binding mode |
| `logLevel` | `2` | klog verbosity (`0`=errors only, `2`=info, `4`=debug) |
| `controller.replicas` | `1` | Number of controller replicas (keep 1 for lab use) |
| `sidecars.provisioner.image` | `…csi-provisioner:v5.2.0` | external-provisioner sidecar image |
| `sidecars.attacher.image` | `…csi-attacher:v4.8.1` | external-attacher sidecar image |
| `sidecars.registrar.image` | `…csi-node-driver-registrar:v2.13.0` | node-driver-registrar sidecar image |

### Driver CLI flags / environment variables

The driver binary accepts the following flags (each also readable from the corresponding
environment variable):

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--mode` | `MODE` | `controller` | `controller` or `node` |
| `--driver-name` | `DRIVER_NAME` | `simple-local-path.csi.whity.ch` | CSI driver name |
| `--endpoint` | `CSI_ENDPOINT` | `unix:///csi/csi.sock` | gRPC socket endpoint |
| `--base-path` | `BASE_PATH` | `/srv/k3d-persistent-volumes` | Host base path |
| `--node-id` | `NODE_ID` | _(empty)_ | Node identifier (injected from `spec.nodeName`) |
| `--stale-timeout` | `STALE_TIMEOUT` | `1m` | Stale publish lock timeout |
| `--allow-force-takeover` | `ALLOW_FORCE_TAKEOVER` | `false` | Force takeover flag |
| `--log-level` | — | `2` | klog verbosity |

---

## Usage Example

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: my-data
  namespace: default
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: simple-local-path
  resources:
    requests:
      storage: 1Gi
```

This creates the backing directory `/srv/k3d-persistent-volumes/default-my-data`.

### StatefulSet example

```yaml
volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: [ReadWriteOnce]
      storageClassName: simple-local-path
      resources:
        requests:
          storage: 1Gi
```

A StatefulSet named `db` with 3 replicas creates:

```
/srv/k3d-persistent-volumes/default-data-db-0
/srv/k3d-persistent-volumes/default-data-db-1
/srv/k3d-persistent-volumes/default-data-db-2
```

---

## Data Persistence Across Cluster Recreation

1. Delete the k3d cluster: `k3d cluster delete my-cluster`
2. Recreate the cluster, mounting the same host path:
   ```bash
   k3d cluster create my-cluster \
     --volume /srv/k3d-persistent-volumes:/srv/k3d-persistent-volumes@all
   ```
3. Reinstall the driver: `helm install ...`
4. Redeploy your workloads with the same PVC names.

The driver will find the existing backing directories and reuse them. Pod data is intact.

---

## Volume Ownership and Stale Lock Recovery

The driver enforces single-node writable publication. When a volume is published on a
node, it writes the node identity and timestamp into `.csi-meta.json` inside the backing
directory.

**Normal flow:**

1. Pod scheduled on node A → `ControllerPublishVolume` records node A as owner.
2. Pod deleted → `ControllerUnpublishVolume` clears the owner.
3. Pod rescheduled on node B → node B can now acquire ownership.

**Stale lock recovery:**

If node A dies without a clean unpublish, the lock remains. The next publish attempt
from another node will:

- **Fail** if `staleTimeout=0` or the lock age is less than `staleTimeout`.
- **Succeed with a warning** if the lock is older than `staleTimeout` (default: `1m`).
- **Succeed immediately** if `allowForceTakeover=true`.

To manually recover a stuck volume, either:

```bash
# Option 1: edit the metadata file directly on the host
cat /srv/k3d-persistent-volumes/default-my-data/.csi-meta.json
# set publishedNode to "" and save

# Option 2: enable force takeover temporarily
helm upgrade simple-local-path deploy/helm/simple-local-path-provisioner \
  --set allowForceTakeover=true
# ... wait for pod to reschedule and mount successfully ...
helm upgrade simple-local-path deploy/helm/simple-local-path-provisioner \
  --set allowForceTakeover=false
```

---

## Uninstall

```bash
# Remove the Helm release (StorageClass and RBAC are removed)
helm uninstall simple-local-path --namespace simple-local-path

# Backing directories on the host are NOT removed automatically.
# Delete them manually if no longer needed:
rm -rf /srv/k3d-persistent-volumes/
```

> PVs and PVCs created by the driver should be deleted before uninstalling Helm,
> otherwise they remain in a `Released`/`Lost` state.

---

## Makefile Targets

| Target | Description |
|---|---|
| `make build` | Build the driver binary to `bin/driver` |
| `make tidy` | Run `go mod tidy` |
| `make docker-build` | Build the Docker image |
| `make helm-lint` | Lint the Helm chart |
