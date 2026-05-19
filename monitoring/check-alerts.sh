#!/bin/bash
# Validates Prometheus alert rules syntax
# Usage: ./check-alerts.sh [rules-file]
#
# If no rules file is provided, defaults to monitoring/prometheus/rules.yml

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default rules file
RULES_FILE="${1:-monitoring/prometheus/rules.yml}"

echo "========================================"
echo "DSN Alert Rules Validation"
echo "========================================"
echo ""

# Check if file exists
if [ ! -f "$RULES_FILE" ]; then
    echo -e "${RED}ERROR: Rules file not found: $RULES_FILE${NC}"
    exit 1
fi

echo "Checking rules file: $RULES_FILE"
echo ""

# Check YAML syntax first (basic validation)
echo "Step 1: Checking YAML syntax..."
if command -v python3 &> /dev/null; then
    if python3 -c "import yaml; yaml.safe_load(open('$RULES_FILE'))" 2>/dev/null; then
        echo -e "${GREEN}✓ YAML syntax valid${NC}"
    else
        echo -e "${RED}✗ YAML syntax invalid${NC}"
        exit 1
    fi
elif command -v python &> /dev/null; then
    if python -c "import yaml; yaml.safe_load(open('$RULES_FILE'))" 2>/dev/null; then
        echo -e "${GREEN}✓ YAML syntax valid${NC}"
    else
        echo -e "${RED}✗ YAML syntax invalid${NC}"
        exit 1
    fi
else
    echo -e "${YELLOW}⚠ python not available - skipping YAML validation${NC}"
fi
echo ""

# Try promtool if available
echo "Step 2: Validating with promtool..."
if command -v promtool &> /dev/null; then
    if promtool check rules "$RULES_FILE"; then
        echo -e "${GREEN}✓ Alert rules validated with promtool${NC}"
    else
        echo -e "${RED}✗ Alert rules validation failed${NC}"
        exit 1
    fi
else
    echo -e "${YELLOW}⚠ promtool not installed - skipping advanced validation${NC}"
    echo ""
    echo "To install promtool:"
    echo "  • Download from: https://github.com/prometheus/prometheus/releases"
    echo "  • Or via Go: go install github.com/prometheus/prometheus/cmd/promtool@latest"
    echo ""
    echo "Alternatively, run Prometheus in dry-run mode to validate rules."
fi

echo ""
echo "========================================"
echo -e "${GREEN}✅ Alert rules validation complete!${NC}"
echo "========================================"
echo ""
echo "Summary:"
echo "  - Rules file: $RULES_FILE"
echo "  - Format: Prometheus YAML"
echo ""

# Optional: List alert names
echo "Configured alerts:"
if command -v yq &> /dev/null; then
    yq e '.groups[].rules[].alert // empty' "$RULES_FILE" 2>/dev/null | grep -v "^$" | sed 's/^/  - /'
elif command -v python3 &> /dev/null; then
    python3 -c "
import yaml
with open('$RULES_FILE') as f:
    data = yaml.safe_load(f)
    alerts = []
    for group in data.get('groups', []):
        for rule in group.get('rules', []):
            if 'alert' in rule:
                alerts.append(rule['alert'])
    for a in sorted(alerts):
        print(f'  - {a}')
"
fi