# Simple Skyhook Test

## Purpose

A basic test that validates the core skyhook functionality with a simple package deployment.

## Test Scenario

1. Reset state from previous runs
2. Apply a LimitRange in the namespace
3. Apply a simple skyhook with basic packages
4. Wait for the skyhook to complete
5. Assert the node and skyhook state are correct

## Key Features Tested

- Basic skyhook creation and processing
- Package deployment to nodes
- Node status and annotations
- LimitRange compatibility
- Skyhook completion

## Files

- `chainsaw-test.yaml` - Main test configuration
- `skyhook.yaml` - Simple skyhook definition
- `limitrange.yaml` - Namespace LimitRange
- `assert.yaml` - State assertions
