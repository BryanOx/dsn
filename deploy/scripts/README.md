# Helm Validation Scripts

This directory contains scripts to validate the DSN Helm chart across different configurations and deployment modes.

## validate-helm.sh

The main validation script that performs comprehensive Helm chart validation:

### Checks Performed

1. **Helm Lint** - Validates chart syntax and best practices
2. **Template (default)** - Renders chart with default values
3. **Template (staging)** - Renders chart with staging-specific configuration:
   - 3 replicas
   - StatefulSet enabled
   - PVC enabled
   - Staging chain ID
4. **Template (production)** - Renders chart with production configuration:
   - 5 replicas
   - StatefulSet enabled
   - PVC with fast-ssd storage
   - Production chain ID
   - Custom resources
5. **Dry-run** - Client-side dry-run (no cluster required)
6. **StatefulSet validation** - Confirms StatefulSet resources are generated when enabled
7. **Deployment validation** - Confirms Deployment resources are generated when disabled

### Usage

```bash
# Make executable (Linux/macOS)
chmod +x validate-helm.sh

# Run validation
./validate-helm.sh

# Or run with bash
bash validate-helm.sh
```

### Requirements

- Helm 3.x installed
- No Kubernetes cluster required (client-side validation)
- Bash 4.x+

### Exit Codes

- `0` - All validations passed
- `1` - Validation failed

## Staging vs Production Configuration

| Parameter | Staging | Production |
|-----------|---------|-------------|
| Replicas | 3 | 5 |
| Storage | 50Gi standard | 100Gi fast-ssd |
| Chain ID | dsn-staging | dsn-1 |
| Resources | Default | Custom (8Gi, 4CPU) |

## Chart Modes

### StatefulSet Mode (Default for Persistence)
```yaml
statefulset:
  enabled: true
persistence:
  enabled: true
```
- Generates StatefulSet with stable network identity
- Generates PVC templates for persistent storage
- Suitable for nodes requiring stable storage

### Deployment Mode (Stateless)
```yaml
statefulset:
  enabled: false
persistence:
  enabled: false
```
- Generates Deployment
- No persistent storage
- Suitable for read-only or validator nodes with external storage

## Integration with CI/CD

Add to your CI pipeline:

```yaml
# .gitlab-ci.yml
validate-helm:
  stage: validate
  script:
    - bash deploy/scripts/validate-helm.sh
  only:
    - merge_requests
    - main
```

```yaml
# .github/workflows/validate.yml
- name: Validate Helm Chart
  run: bash deploy/scripts/validate-helm.sh
```