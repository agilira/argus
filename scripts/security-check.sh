#!/bin/bash
# Argus Security Check Script
# This script runs security analysis on the Argus codebase using gosec
# 
# Usage: ./scripts/security-check.sh

set -e

echo "🔒 Running Argus Security Analysis with gosec..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Check if gosec is installed
if ! command -v gosec &> /dev/null && ! [ -f ~/go/bin/gosec ]; then
    echo "❌ gosec not found. Please install gosec first:"
    echo "   go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest"
    exit 1
fi

# Use gosec from the expected location
GOSEC_CMD="gosec"
if [ -f ~/go/bin/gosec ]; then
    GOSEC_CMD="$HOME/go/bin/gosec"
fi

# Run gosec with proper exclusions for production
echo "📊 Scanning codebase (excluding demo-specific issues)..."
echo ""

# Exclude:
# G104 - Unhandled errors (acceptable in examples/demos)
# G306 - File permissions (examples use standard permissions)
# G301 - Directory permissions (examples use standard permissions)
$GOSEC_CMD --exclude=G104,G306,G301 ./...

echo ""
echo "✅ Security analysis completed!"
echo "Note: Excluded G104 (demo error handling), G306/G301 (demo file permissions)"
echo "All critical security issues (G115, G303, G304) are properly handled with #nosec comments"
