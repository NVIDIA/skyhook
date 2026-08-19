# Simple NodeWright Test

## Purpose

A basic test that validates the core nodewright functionality with a simple package deployment.

## Test Scenario

1. Reset state from previous runs
2. Apply a LimitRange in the namespace
3. Apply a simple nodewright with basic packages
4. Wait for the nodewright to complete
5. Assert the node and nodewright state are correct

## Key Features Tested

- Basic nodewright creation and processing
- Package deployment to nodes
- Node status and annotations
- LimitRange compatibility
- NodeWright completion

## Files

- `chainsaw-test.yaml` - Main test configuration
- `nodewright.yaml` - Simple nodewright definition
- `limitrange.yaml` - Namespace LimitRange
- `assert.yaml` - State assertions
