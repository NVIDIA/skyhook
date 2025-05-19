# Skyhook Documentation

This directory contains user and operator documentation for Skyhook. Here you'll find guides, examples, and reference material to help you deploy, configure, and secure Skyhook in your Kubernetes cluster.

## Available Documentation

- [Resource Management](resource_management.md):
  How Skyhook manages CPU/memory resources using LimitRange, per-package overrides, and validation rules.

- [Kyverno Policy Examples](kyverno/README.md):
  Example Kyverno policies for restricting images or packages in Skyhook resources.

- [Providing Secrets to Packages](providing_secrets_to_packages.md):
  How to securely provide secrets to Skyhook-managed packages.

- [Releases](releases.md):
  Release notes and upgrade information for Skyhook.

- [Runtime Required](runtime_required.md):
  How to use the runtime required taint and feature in Skyhook.
