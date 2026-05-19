# Staging Deployment - DSN 3-Validator Cluster

Kubernetes staging configuration for deploying a 3-validator DSN cluster.

## Prerequisites

- Kubernetes cluster (minikube, kind, or cloud K8s)
- Helm v3
- kubectl
- kustomize (optional, for kustomize build approach)

## Quick Start

### Option 1: Helm + Kustomize (Recommended)

```bash
# 1. Generate Helm template output as base
helm template charts/dsn/ > deploy/staging/base.yaml

# 2. Build and apply the staging overlay
kustomize build deploy/staging/ | kubectl apply -f -

# 3. Verify deployment
kubectl -n dsn-staging get pods
kubectl -n dsn-staging get pvc
```

### Option 2: Helm Only (Simpler)

```bash
# 1. Create a staging values file
cp charts/dsn/values.yaml charts/dsn/values-staging.yaml

# 2. Edit values-staging.yaml:
#    - replicaCount: 3
#    - statefulset.enabled: true
#    - persistence.enabled: true
#    - persistence.storageClass: "fast-ssd"
#    - persistence.size: 20Gi

# 3. Install with staging values
helm install dsn-staging charts/dsn/ -f charts/dsn/values-staging.yaml --namespace dsn-staging --create-namespace

# 4. Port forward to access services
kubectl -n dsn-staging port-forward svc/dsn 8545:8545
```

## Verify Deployment

```bash
# Check pods
kubectl -n dsn-staging get pods -l app.kubernetes.io/component=validator

# Check PVCs
kubectl -n dsn-staging get pvc

# Check services
kubectl -n dsn-staging get svc

# View logs
kubectl -n dsn-staging logs -l app.kubernetes.io/component=validator

# Port forward to Grafana (if installed via monitoring stack)
kubectl -n dsn-staging port-forward svc/dsn-grafana 3000:3000
```

## Configuration

### Resource Limits

- **Requests**: 2 CPU, 4Gi memory
- **Limits**: 4 CPU, 8Gi memory
- **Storage**: 20Gi per validator (PVC)

### Network Access

| Port | Service | Description |
|------|---------|-------------|
| 30333 | P2P | Validator P2P networking |
| 8545 | RPC | JSON-RPC API |
| 9090 | Metrics | Prometheus metrics endpoint |
| 26656 | Tendermint P2P | Internal consensus P2P |
| 26657 | Tendermint RPC | Internal RPC |

### Bootstrap Peers

Update `bootstrap-configmap.yaml` with actual peer IDs after initial startup:

```bash
# Get peer ID from first validator
kubectl -n dsn-staging exec -it dsn-0 -- dsn tendermint show-node-id
```

Then update ConfigMap:
```bash
kubectl -n dsn-staging edit configmap dsn-bootstrap
```

## Troubleshooting

### Pods not starting

```bash
# Check events
kubectl -n dsn-staging describe pod dsn-0

# Check storage class exists
kubectl get storageclass fast-ssd
```

### Network policy blocking traffic

```bash
# Check network policies
kubectl -n dsn-staging get networkpolicy

# Test connectivity between pods
kubectl -n dsn-staging exec -it dsn-0 -- curl -v http://dsn-1.dsn-staging.svc.cluster.local:8545
```

### PVC stuck in Pending

```bash
# Check storage class binding mode
kubectl get storageclass fast-ssd -o yaml

# For local clusters, ensure volume binding is set to WaitForFirstConsumer
```

## Cleanup

```bash
# Delete staging namespace (removes all resources)
kubectl delete namespace dsn-staging
```

## File Structure

```
deploy/staging/
├── kustomization.yaml         # Kustomize overlay configuration
├── namespace.yaml             # dsn-staging namespace definition
├── bootstrap-configmap.yaml   # Bootstrap peer configuration
├── pvc-storage-class.yaml    # Fast SSD storage class
├── network-policy.yaml       # Network isolation rules
└── README.md                 # This file
```

## Next Steps

1. **Configure Monitoring**: Install Prometheus + Grafana via `monitoring/` charts
2. **Update Bootstrap**: Replace placeholder peer IDs in ConfigMap
3. **Verify Consensus**: Check validator logs for block finalization
4. **Run Tests**: Execute soak tests from `integration/soak/`