# Strict Order Test

## Purpose

Validates per-node priority ordering where nodes process skyhooks in priority order **independently**. This test proves that the ordering is **per-node, not global** - one node can progress through multiple priorities while another node is still on an earlier priority.

## Test Scenario

The test uses **2 nodes throughout** to validate per-node behavior for all features (priority, pause, disable, waiting):

### Phase 1: Setup and Block Node 2
1. Reset state from previous runs
2. Label both `kind-worker` and `kind-worker2` with `strict-order-test=true`
3. Temporarily block `kind-worker2` with `test-block=true:NoSchedule` taint (to create timing difference)

### Phase 2: Apply Skyhooks (Both Nodes Targeted, Worker Runs First)
4. Apply skyhooks:
   - Priority 1: `strict-order-skyhook-zzz` (not paused, not disabled)
   - Priority 2: `strict-order-skyhook-b` (paused initially)
   - Priority 2: `strict-order-skyhook-c` (not paused, not disabled)
   - Priority 2: `strict-order-skyhook-d` (disabled)
5. Validate metrics: 2 nodes targeted per skyhook
6. Result: worker completes priority 1 and reaches priority 2 (paused at b), worker2 can't start (blocked by taint)

### Phase 3: Assert Worker at Priority 2, Worker2 Blocked at Priority 1
7. **KEY ASSERTION with pod checks**:
   - Worker has zzz pods (priority 1 complete) and is paused at b (priority 2)
   - Worker2 has NO pods yet (blocked by taint at priority 1)
   - This proves worker reached priority 2 while worker2 stuck at priority 1
8. Validate metrics: per-node status counts showing one node complete, one blocked

### Phase 4: Unpause and Unblock Simultaneously (CRITICAL TEST)
9. Unpause `strict-order-skyhook-b` via patch
10. Remove `test-block` taint from worker2 in same step
11. Both nodes now running concurrently at different priorities

### Phase 5: Assert Concurrent Different Priorities (CRITICAL)
12. **CRITICAL ASSERTION with pod checks**:
   - Worker has pods for priority 2 skyhooks (b or c)
   - Worker2 has pods for priority 1 skyhook (zzz)
   - **This definitively proves per-node ordering**: worker is ahead in priority queue during concurrent execution
13. In old global ordering, ALL nodes would need to complete priority 1 before ANY node could start priority 2

### Phase 6: Assert Both Nodes Complete
14. Wait for both nodes to complete all skyhooks
15. Assert final state: both nodes have zzz, b, c complete; d disabled
16. Validate metrics: 2 nodes complete for each skyhook

## Key Features Tested

- **Per-node priority ordering** (nodes don't wait for each other between priorities)
- **Concurrent execution at different priorities** (proves ordering is per-node, not global)
- **Alphabetical ordering for same-priority skyhooks** (b before c at priority 2)
- **Per-node pause behavior** (both nodes pause at priority 2, then both unpause independently)
- **Per-node disable behavior** (disabled skyhook doesn't block other skyhooks on any node)
- **Per-node waiting status** (based on completion of higher-priority skyhooks on THAT node)
- **Blocked status** (node can't start due to external conditions like taints)

## Files

- `chainsaw-test.yaml` - Main test configuration with 6 phases
- `skyhook.yaml` - Multiple skyhooks with different priorities targeting both nodes
- `skyhook-pause-update.yaml` - Skyhooks with pause annotation removed
- `skyhook-disable-update.yaml` - Skyhooks with disable annotation (optional, not used in current test)
- `assert-node1-priority1-complete-node2-blocked.yaml` - Phase 3 assertion: worker at priority 2 (paused), worker2 blocked at priority 1
- `assert-concurrent-different-priorities.yaml` - Phase 5 CRITICAL assertion: worker completed priority 1, worker2 still on priority 1 (proves per-node ordering via node annotations)
- `assert-multiple-skyhooks-in-progress.yaml` - Phase 5 CRITICAL assertion: zzz (priority 1) and b (priority 2) both in_progress simultaneously (proves concurrent execution at different priorities - impossible in old global ordering)
- `assert-both-nodes-complete.yaml` - Phase 6 assertion: both nodes complete all skyhooks

## Notes

- Uses dedicated label `skyhook.nvidia.com/strict-order-test=true` on both worker nodes throughout entire test
- Worker2 is blocked only temporarily to create timing difference, not excluded from test
- This is fundamentally a **per-node ordering test** using 2 nodes throughout
- **Critical proof**: Pod assertions show worker running priority 2 pods while worker2 runs priority 1 pods concurrently
