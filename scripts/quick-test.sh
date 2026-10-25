#!/bin/bash
# Quick test script to demonstrate Makefile features

set -e

echo "🚀 Echo Observatory Quick Test Script"
echo "============================"
echo ""

# Clean up first
echo "📦 Cleaning up..."
make clean

# Show project info
echo ""
echo "ℹ️  Project Information:"
make info

# Quick build
echo ""
echo "🔨 Building application..."
make build-quick

# Run quick tests
echo ""
echo "🧪 Running short tests..."
make test-short

# Show available commands
echo ""
echo "📋 Available commands:"
echo "  make run          - Run with default settings"
echo "  make run-all      - Run with all protocols"
echo "  make run-debug    - Run in debug mode"
echo "  make test-integration - Run full integration tests"
echo ""
echo "✅ Quick test completed!"