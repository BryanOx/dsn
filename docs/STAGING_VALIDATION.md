# DSN v0.1.0-sandbox — Staging Validation

This document provides comprehensive validation procedures for deploying a 3-validator DSN cluster in a Kubernetes staging environment.

## Prerequisites

- Kubernetes cluster (minikube, kind, or cloud)
- Helm v3
- kubectl
- Go 1.21+

### Required Tools Check

```bash
# Verify Kubernetes access
kubectl cluster-info

# Verify Helm installation
helm version

# Verify Go version
go version
```

## Deployment Steps

### 1. Deploy Helm Chart

Option A: Direct Helm install with staging values:

```bash
# Create staging values file
cat > values-staging.yaml << 'EOF'
replicaCount: 3
statefulset:
  enabled: true
persistence:
  enabled: true
  storageClass: "standard"
  size: 50Gi
config:
  chain:
    id: "dsn-staging"
  p2p:
    seeds: "seed1@dsn-seed-1:26656,seed2@dsn-seed-2:26656"
  metrics:
    enabled: true
resources:
  limits:
    cpu: 4000m
    memory: 8Gi
  requests:
    cpu: 1000m
    memory: 2Gi
EOF

# Install the chart
helm install dsn-sandbox charts/dsn/ \
  --namespace dsn-sandbox \
  --create-namespace \
  -f values-staging.yaml
```

Option B: Kustomize overlay (recommended):

```bash
# Generate Helm template output as base
helm template charts/dsn/ > deploy/staging/base.yaml

# Build and apply the staging overlay
kustomize build deploy/staging/ | kubectl apply -f -
```

### 2. Verify Deployment

```bash
# Check pods
kubectl -n dsn-sandbox get pods -l app.kubernetes.io/component=validator

# Check PVCs
kubectl -n dsn-sandbox get pvc

# Check services
kubectl -n dsn-sandbox get svc

# View logs
kubectl -n dsn-sandbox logs -l app.kubernetes.io/component=validator

# Verify pod events if issues arise
kubectl -n dsn-sandbox describe pod dsn-validator-0
```

### 3. Verify Monitoring

Deploy the monitoring stack:

```bash
# Apply monitoring configuration
kubectl apply -f deploy/staging/monitoring.yaml

# Check monitoring pods
kubectl -n dsn-monitoring get pods

# Port-forward Grafana
kubectl -n dsn-monitoring port-forward svc/grafana 3000:3000
```

Access Grafana at `http://localhost:3000` (default credentials: admin/admin)

### 4. Validate Helm Chart

Run the validation script:

```bash
# Make script executable
chmod +x deploy/scripts/validate-helm.sh

# Run validation
./deploy/scripts/validate-helm.sh
```

This validates:
- Helm lint
- Template rendering (default, staging, production)
- StatefulSet and PVC templates
- Deployment templates

## Validation Tests

### Test 1: Rolling Restart

**Purpose**: Validate that individual validator restarts maintain cluster consensus and quorum.

**Procedure**:
1. Record baseline state root before any restarts
2. Restart validator-0, verify it rejoins consensus
3. Produce 2+ blocks while validator-0 is down
4. Restart validator-1, verify catch-up
5. Restart validator-2, verify state convergence

**Expected**:
- Cluster maintains quorum (2/3 validators active)
- No rollback of confirmed blocks
- State roots converge across all validators

**Validation Commands**:
```bash
# Restart individual pods
kubectl -n dsn-sandbox delete pod dsn-validator-0

# Check pod recreation
kubectl -n dsn-sandbox get pods -w

# Verify block production continues
kubectl -n dsn-sandbox logs dsn-validator-1 | grep -i "block"
```

**Checklist**:
- [ ] validator-0 restarted → still producing blocks
- [ ] validator-1 restarted → still producing blocks
- [ ] validator-2 restarted → all nodes synced

### Test 2: PVC Persistence

**Purpose**: Validate that state persists across pod deletions and recreations (StatefulSet behavior).

**Procedure**:
1. Record pre-deletion state root
2. Delete validator pod (PVC should persist)
3. Wait for StatefulSet to recreate pod
4. Verify state root matches pre-deletion

**Expected**:
- PVC binds to new pod automatically
- State root matches before/after deletion
- All WAL entries replayed correctly
- Snapshot at same height available

**Validation Commands**:
```bash
# Get PVC before deletion
kubectl -n dsn-sandbox get pvc

# Delete pod (not PVC)
kubectl -n dsn-sandbox delete pod dsn-validator-0

# Verify PVC still bound
kubectl -n dsn-sandbox get pvc -l app.kubernetes.io/name=dsn

# Check state in persistent volume
kubectl -n dsn-sandbox exec dsn-validator-0 -- ls -la /data
```

**Checklist**:
- [ ] Pod deleted, recreated with same PVC
- [ ] State root matches before/after deletion
- [ ] All blocks processed
- [ ] WAL replayed from persisted files

### Test 3: Network Partition Recovery

**Purpose**: Validate validator can re-sync after being isolated from the network.

**Procedure**:
1. Record pre-partition state root
2. Induce partition (block P2P port 30333 on one validator)
3. Let other validators produce 10+ blocks
4. Heal partition (unblock port)
5. Verify partitioned validator catches up

**Expected**:
- Partitioned validator has pre-partition state
- After healing, validator catches up to chain tip
- State roots converge

**Validation Commands**:
```bash
# Simulate partition - block P2P port (use NetworkPolicy or firewall)
# In Kubernetes, apply network policy:

apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: partition-validator-2
  namespace: dsn-sandbox
spec:
  podSelector:
    matchLabels:
      statefulset.kubernetes.io/pod-name: dsn-validator-2
  policyTypes:
    - Ingress
    - Egress

# Wait and let other validators produce blocks
sleep 60

# Check block height on other validators
kubectl -n dsn-sandbox exec dsn-validator-0 -- dsn status

# Remove NetworkPolicy to heal partition
kubectl delete -f partition-validator-2.yaml

# Verify catch-up
kubectl -n dsn-sandbox exec dsn-validator-2 -- dsn status
```

**Checklist**:
- [ ] Partition induced (block port 30333)
- [ ] 10+ blocks produced by other validators
- [ ] Partition healed (unblock port)
- [ ] Partitioned validator catches up

### Test 4: Snapshot Recovery

**Purpose**: Validate node can recover from snapshot after state deletion.

**Procedure**:
1. Take snapshot backup of current state
2. Delete state data on one validator
3. Restart node and trigger snapshot restore
4. Verify correct state recovery and chain sync

**Expected**:
- Snapshot file exists and is valid
- Node recovers to snapshot height
- Continues block production from snapshot point

**Validation Commands**:
```bash
# Create snapshot backup
kubectl -n dsn-sandbox exec dsn-validator-0 -- dsn snapshot backup

# Verify snapshot created
kubectl -n dsn-sandbox exec dsn-validator-0 -- ls /data/snapshots/

# Delete state (simulate corruption)
kubectl -n dsn-sandbox exec dsn-validator-0 -- rm -rf /data/state/*
kubectl -n dsn-sandbox exec dsn-validator-0 -- rm -rf /data/wal/*

# Restart validator to trigger recovery
kubectl -n dsn-sandbox delete pod dsn-validator-0

# Check recovery
kubectl -n dsn-sandbox logs dsn-validator-0 | grep -i snapshot
```

**Checklist**:
- [ ] Snapshot backup taken
- [ ] State deleted
- [ ] Snapshot restored
- [ ] Node syncs correctly

### Test 5: Crash Recovery

**Purpose**: Validate node can recover from abrupt process termination (SIGKILL).

**Procedure**:
1. Record current state root and height
2. Send SIGKILL to validator process
3. Restart validator process
4. Verify WAL replay and state recovery

**Expected**:
- Uncommitted WAL entries are replayed
- State root matches pre-crash after recovery
- Node catches up to current chain

**Validation Commands**:
```bash
# Find validator process
PID=$(kubectl -n dsn-sandbox exec dsn-validator-0 -- pgrep dsn)

# Kill the process (simulate crash)
kubectl -n dsn-sandbox exec dsn-validator-0 -- kill -9 $PID

# Wait for restart
sleep 10

# Check recovery in logs
kubectl -n dsn-sandbox logs dsn-validator-0 | grep -i "wal\|replay\|recovery"

# Verify state root
kubectl -n dsn-sandbox exec dsn-validator-0 -- dsn status
```

**Checklist**:
- [ ] SIGKILL validator process
- [ ] Process restarted
- [ ] WAL replayed
- [ ] State root matches

## Validation Results

| Test | Status | Duration | Notes |
|------|--------|----------|-------|
| Rolling Restart | ⬜ | — | — |
| PVC Persistence | ⬜ | — | — |
| Network Partition | ⬜ | — | — |
| Snapshot Recovery | ⬜ | — | — |
| Crash Recovery | ⬜ | — | — |

## Metrics Validation

### Prometheus Targets

```bash
# Port-forward Prometheus
kubectl -n dsn-monitoring port-forward svc/prometheus 9090:9090

# Check targets
curl http://localhost:9090/api/v1/targets | jq '.data.activeTargets[] | select(.labels.job == "dsn-validators")'
```

**Checklist**:
- [ ] Prometheus targets are UP (check /targets)
- [ ] Grafana dashboard shows block height increasing
- [ ] Alerting rules evaluate correctly
- [ ] Node-exporter metrics available
- [ ] Logs are being collected

### Alert Rules Verification

Verify the following alerts are configured in `monitoring/prometheus/rules.yml`:

| Alert | Expression | Severity |
|-------|------------|----------|
| DSNBlockHeightNotIncreasing | `rate(dsn_block_height[5m]) == 0` | critical |
| DSNSyncLagHigh | `dsn_node_sync_lag_blocks > 10` | warning |
| DSNPeerCountLow | `dsn_p2p_peer_count < 3` | warning |
| DSNValidatorDown | `up{job="dsn"} == 0` | critical |
| DSNCPUUsageHigh | `rate(process_cpu_seconds_total[5m]) > 0.8` | warning |
| DSNMemoryUsageHigh | `process_resident_memory_bytes / 4294967296 > 0.9` | warning |
| PanicRecoveryRate | `rate(dsn_panic_recovery_total[5m]) > 0` | critical |
| GoroutineSpike | `go_goroutines > 10000` | warning |
| MempoolPressure | `dsn_mempool_current_size / dsn_mempool_max_size > 0.8` | warning |

### Grafana Dashboard

Access `monitoring/grafana/dsn-dashboard.json` for panels:

- Block Height (stat panel)
- Peer Count (stat panel)
- Active Validators (stat panel)
- Block Time (timeseries)
- CPU Usage (timeseries)
- Memory Usage (timeseries)
- Consensus Round (timeseries)
- Sync Lag (timeseries)

## Infrastructure Validation

### Core Components

```bash
# Check all validator pods are running
kubectl -n dsn-sandbox get pods -l app.kubernetes.io/component=validator

# Verify persistent volumes are bound
kubectl -n dsn-sandbox get pvc

# Check services are exposed
kubectl -n dsn-sandbox get svc
```

**Checklist**:
- [ ] 3 validators in active set
- [ ] Persistent volumes bound
- [ ] RPC endpoint reachable
- [ ] P2P peering established
- [ ] Explorer indexing blocks
- [ ] Faucet operational

### Port Reference

| Port | Service | Description |
|------|---------|-------------|
| 30333 | P2P | Validator P2P networking |
| 8545 | RPC | JSON-RPC API |
| 9090 | Metrics | Prometheus metrics endpoint |
| 26656 | Tendermint P2P | Internal consensus P2P |
| 26657 | Tendermint RPC | Internal RPC |

### Network Connectivity

```bash
# Test P2P connectivity between validators
kubectl -n dsn-sandbox exec dsn-validator-0 -- curl -v http://dsn-validator-1.dsn-staging.svc.cluster.local:26657/status

# Test RPC accessibility
kubectl -n dsn-sandbox exec dsn-validator-0 -- curl -v http://localhost:8545/status

# Test metrics endpoint
kubectl -n dsn-sandbox exec dsn-validator-0 -- curl -v http://localhost:9090/metrics
```

## Integration Test Reference

The following Go integration tests in `integration/` provide automated validation:

### Staging Tests (`integration/staging/`)
- `TestPVC_PersistenceAcrossRestart` - Validates state persists across pod restarts
- `TestPVC_WALReplayFromPersistedFiles` - Validates WAL replay from PVC
- `TestPVC_StateRootConvergence` - Validates state roots converge after restart
- `TestRollingRestart_Convergence` - Validates rolling restart maintains consensus
- `TestRollingRestart_QuorumMaintenance` - Validates quorum during restarts

### Resilience Tests (`integration/resilience_test.go`)
- `TestResilience_RollingRestart` - T4-1: Graceful rolling restart
- `TestResilience_CrashLoopRecovery` - T4-2: Crash-loop recovery
- `TestResilience_CorruptedSnapshot` - T4-3: Corrupted snapshot recovery
- `TestResilience_NetworkPartition` - T4-4: Network partition recovery
- `TestResilience_KillRestartLoop` - T4-5: Kill/restart loop (10 iterations)

Run tests:
```bash
go test -v -tags=integration ./integration/staging/...
go test -v -tags=integration ./integration/... -run TestResilience
```

## Troubleshooting

### Pods Not Starting

```bash
# Check events
kubectl -n dsn-sandbox describe pod dsn-validator-0

# Check storage class
kubectl get storageclass standard

# For local clusters, ensure WaitForFirstConsumer binding
kubectl get storageclass standard -o yaml | grep volumeBindingMode
```

### PVC Stuck in Pending

```bash
# Check storage class binding mode
kubectl get storageclass

# Verify storage provisioner is available
kubectl get pods -n kube-system | grep -i storage
```

### Network Policy Blocking Traffic

```bash
# Check network policies
kubectl -n dsn-sandbox get networkpolicy

# Test connectivity
kubectl -n dsn-sandbox exec -it dsn-validator-0 -- curl -v http://dsn-validator-1.dsn-staging.svc.cluster.local:8545
```

## Cleanup

```bash
# Delete staging namespace (removes all resources)
kubectl delete namespace dsn-staging
kubectl delete namespace dsn-monitoring

# Remove Helm release
helm uninstall dsn-sandbox
```

## Related Documentation

- [OPERATIONS.md](./OPERATIONS.md) - Day-to-day operational procedures
- [RECOVERY_RUNBOOK.md](./RECOVERY_RUNBOOK.md) - Detailed recovery procedures
- [PERSISTENCE.md](./PERSISTENCE.md) - Storage and state management
- [NETWORKING.md](./NETWORKING.md) - P2P networking details