#!/bin/bash
# Helm Validation Script for DSN Chart
# Validates the Helm chart for the DSN node across different configurations

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHART_DIR="$(dirname "$SCRIPT_DIR")/dsn"

echo "=== DSN Helm Validation ==="
echo "Chart directory: $CHART_DIR"
echo ""

# Check if Helm is available
if ! command -v helm &> /dev/null; then
    echo "ERROR: helm is not installed"
    exit 1
fi

echo "=== Helm Version ==="
helm version
echo ""

echo "=== Helm Lint ==="
helm lint "$CHART_DIR/"
echo "Lint: PASSED"
echo ""

echo "=== Helm Template (default values) ==="
helm template dsn-default "$CHART_DIR/" > /dev/null
echo "Template (default): PASSED"
echo ""

echo "=== Helm Template (staging values) ==="
# Create staging values override
STAGING_VALUES=$(cat << 'EOF'
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
EOF
)

# Write staging values to temp file
STAGING_VALUES_FILE=$(mktemp)
echo "$STAGING_VALUES" > "$STAGING_VALUES_FILE"

helm template dsn-staging "$CHART_DIR/" --namespace dsn-staging -f "$STAGING_VALUES_FILE" > /dev/null
echo "Template (staging): PASSED"

rm "$STAGING_VALUES_FILE"
echo ""

echo "=== Helm Template (production values) ==="
# Create production values override
PROD_VALUES=$(cat << 'EOF'
replicaCount: 5
statefulset:
  enabled: true
persistence:
  enabled: true
  storageClass: "fast-ssd"
  size: 100Gi
config:
  chain:
    id: "dsn-1"
  p2p:
    persistentPeers: "peer1@dsn-peer-1:26656,peer2@dsn-peer-2:26656"
  metrics:
    enabled: true
  consensus:
    timeoutPropose: 2s
    timeoutPrecommit: 500ms
resources:
  limits:
    cpu: 4000m
    memory: 8Gi
  requests:
    cpu: 1000m
    memory: 2Gi
EOF
)

PROD_VALUES_FILE=$(mktemp)
echo "$PROD_VALUES" > "$PROD_VALUES_FILE"

helm template dsn-prod "$CHART_DIR/" --namespace dsn-prod -f "$PROD_VALUES_FILE" > /dev/null
echo "Template (production): PASSED"

rm "$PROD_VALUES_FILE"
echo ""

echo "=== Helm Install Dry-Run (staging) ==="
# Note: dry-run requires kubeconfig or --dry-run=client for no cluster
helm template dsn-test "$CHART_DIR/" --namespace dsn-staging --create-namespace > /dev/null
echo "Dry-run: PASSED (client-side)"
echo ""

echo "=== Validate StatefulSet Template ==="
# Extract and validate StatefulSet rendering
OUTPUT=$(helm template dsn-stateful "$CHART_DIR/" --set statefulset.enabled=true --set persistence.enabled=true)
if echo "$OUTPUT" | grep -q "kind: StatefulSet"; then
    echo "StatefulSet: FOUND"
else
    echo "ERROR: StatefulSet not found in output"
    exit 1
fi

# Verify PVC template exists
if echo "$OUTPUT" | grep -q "kind: PersistentVolumeClaim"; then
    echo "PVC Template: FOUND"
else
    echo "ERROR: PVC template not found in output"
    exit 1
fi
echo ""

echo "=== Validate Deployment Template ==="
OUTPUT=$(helm template dsn-deploy "$CHART_DIR/" --set statefulset.enabled=false --set persistence.enabled=false)
if echo "$OUTPUT" | grep -q "kind: Deployment"; then
    echo "Deployment: FOUND"
else
    echo "ERROR: Deployment not found in output"
    exit 1
fi
echo ""

echo "============================================="
echo "=== ALL HELM VALIDATION CHECKS PASSED ==="
echo "============================================="
echo ""
echo "Summary:"
echo "  - Lint: OK"
echo "  - Template (default): OK"
echo "  - Template (staging): OK"
echo "  - Template (production): OK"
echo "  - Dry-run: OK"
echo "  - StatefulSet: OK"
echo "  - Deployment: OK"