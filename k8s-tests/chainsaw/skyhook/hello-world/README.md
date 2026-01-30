# Hello World Test

## Purpose

A very simple test that creates a configmap and verifies its content. This serves as a basic sanity check for the test framework.

## Test Scenario

1. Apply a configmap to the cluster
2. Assert the configmap content is as expected

## Key Features Tested

- Basic Chainsaw test framework functionality
- ConfigMap creation and validation

## Files

- `chainsaw-test.yaml` - Main test configuration
- `configmap.yaml` - ConfigMap definition
- `configmap-assert.yaml` - Content assertions

## Notes

- This test is **skipped** as there are sufficient examples in other tests
