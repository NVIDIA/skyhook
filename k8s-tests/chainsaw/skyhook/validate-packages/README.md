# Validate Packages Test

## Purpose

Validates that the operator correctly validates pods against the package spec and kills non-compliant pods.

## Test Scenario

1. Create packages with incorrect specifications:
   - Bogus image that doesn't exist
   - Too many resources requested
   - Environment variable that causes the container to hang
2. Apply the skyhook with these invalid packages
3. Verify the operator detects the spec mismatches
4. Update the skyhook with corrected specifications
5. Assert that:
   - Invalid pods are killed
   - New pods with correct spec are created
   - Packages complete successfully

## Key Features Tested

- Pod spec validation against package definition
- Automatic pod termination for non-compliant pods
- Recovery from invalid package configurations
- Image validation
- Resource request/limit validation
- Environment variable handling

## Files

- `chainsaw-test.yaml` - Main test configuration
- `skyhook.yaml` - Initial skyhook with invalid packages
- `update.yaml` - Corrected skyhook configuration
- `assert.yaml` - State assertions
