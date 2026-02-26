# Manual Test: License Validation Telemetry (Part 2A — CRDB Emission Side)

This document describes step-by-step manual tests to verify that license
validation telemetry changes work end-to-end on a real CockroachDB cluster.
It is designed to be executed by Claude, running commands one at a time.

## Prerequisites

### P1: Build the cockroach binary

```bash
cd /Users/vishalvaibhav/code/managed-service-fork/cockroach
./dev build short
```

Wait for the build to complete. The binary will be at `./cockroach`.

### P2: Generate test licenses

We need two license strings with the new proto fields populated. To generate
them, temporarily add a test function to `pkg/ccl/utilccl/license_check_test.go`,
run it, capture the output, then remove it.

**Add this function** to the end of `pkg/ccl/utilccl/license_check_test.go`:

```go
func TestGenManualTestLicenses(t *testing.T) {
	licID := uuid.MakeV4()
	orgID := uuid.MakeV4()
	exp := time.Now().Add(365 * 24 * time.Hour).Unix()

	lic1, err := (&licenseccl.License{
		Type:              licenseccl.License_Enterprise,
		ValidUntilUnixSec: exp,
		OrganizationName:  "Manual Test Org",
		Environment:       licenseccl.Production,
		Edition:           licenseccl.ENTERPRISE_EDITION,
		AddOns:            []licenseccl.License_AddOn{licenseccl.DATA_REPLICATION, licenseccl.ADVANCED_COMPLIANCE},
		VcpuEntitled:      16,
		LicenseId:         licID.GetBytes(),
		OrganizationId:    orgID.GetBytes(),
	}).Encode()
	if err != nil {
		t.Fatal(err)
	}

	lic2, err := (&licenseccl.License{
		Type:              licenseccl.License_Evaluation,
		ValidUntilUnixSec: exp,
		OrganizationName:  "Manual Test Org",
		Environment:       licenseccl.PreProduction,
		Edition:           licenseccl.STANDARD,
		AddOns:            []licenseccl.License_AddOn{licenseccl.DATA_REPLICATION},
		VcpuEntitled:      8,
		LicenseId:         uuid.MakeV4().GetBytes(),
		OrganizationId:    orgID.GetBytes(),
	}).Encode()
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("LICENSE_ID=%s\n", licID.String())
	fmt.Printf("ORG_ID=%s\n", orgID.String())
	fmt.Printf("EXPIRY=%d\n", exp)
	fmt.Printf("ENTERPRISE_LIC=%s\n", lic1)
	fmt.Printf("EVAL_LIC=%s\n", lic2)
}
```

Required imports (add if not already present): `"fmt"`, `"time"`,
`"github.com/cockroachdb/cockroach/pkg/util/uuid"`,
`"github.com/cockroachdb/cockroach/pkg/ccl/utilccl/licenseccl"`.

**Run the test:**

```bash
./dev test pkg/ccl/utilccl -f TestGenManualTestLicenses -v --count 1
```

**Capture the output.** It will print lines like:

```
LICENSE_ID=<uuid>
ORG_ID=<uuid>
EXPIRY=<unix_timestamp>
ENTERPRISE_LIC=crl-0-...
EVAL_LIC=crl-0-...
```

Save these values — they are used throughout all tests below. Then **remove**
the `TestGenManualTestLicenses` function from the file (revert the change).

### P3: Clean up any existing test cluster

```bash
pkill -f "cockroach start" 2>/dev/null
sleep 2
# Force-kill any stragglers
pkill -9 -f "cockroach start" 2>/dev/null
rm -rf /tmp/crdb-node1 /tmp/crdb-node2 /tmp/crdb-node3
```

---

## Test 2A.3: No License — Virtual Table Defaults

**Purpose:** Verify that `crdb_internal.node_license_status` returns correct
defaults when no license is installed.

**Why single-node:** This test only checks the virtual table schema and
default values. A single-node cluster is sufficient and simpler.

### Steps

1. Start a single-node cluster:

```bash
./cockroach start-single-node --insecure --store=/tmp/crdb-node1 \
  --listen-addr=localhost:26257 --http-addr=localhost:8181 --background
```

2. Query the virtual table:

```bash
./cockroach sql --insecure -e \
  "SET allow_unsafe_internals = true; SELECT * FROM crdb_internal.node_license_status;"
```

Note: `SET allow_unsafe_internals` and `SELECT` are two separate statements in
the same `-e` string. CockroachDB executes them sequentially (not as a
multi-statement transaction) in this context.

### Expected Output

| Column | Expected Value |
|--------|---------------|
| has_license | `f` (false) |
| license_type | `none` |
| edition | empty string |
| add_ons | empty string |
| vcpu_entitled | `0` |
| vcpu_observed | system vCPU count (e.g. `12`) |
| expiration | `NULL` |
| grace_period_end | some date (~7 days from now, the default grace period) |
| requires_telemetry | `f` (false) |
| is_disabled | `t` (true, because single-node auto-disables enforcement) |

### Pass Criteria

- All columns exist and have the expected default values.
- `has_license` is `false`, `license_type` is `none`, `edition` and `add_ons`
  are empty.

### Cleanup

```bash
pkill -f "cockroach start" 2>/dev/null; sleep 2
rm -rf /tmp/crdb-node1
```

---

## Test 2A.1: Install License — Virtual Table Verification

**Purpose:** Verify that after setting a license with the new fields, the
virtual table shows all values correctly.

**Why 3-node cluster:** Multi-node clusters do not auto-disable license
enforcement (`is_disabled=false`), which is the production-relevant behavior.

### Steps

1. Start a 3-node cluster:

```bash
./cockroach start --insecure --store=/tmp/crdb-node1 \
  --listen-addr=localhost:26257 --http-addr=localhost:8181 \
  --join=localhost:26257,localhost:26258,localhost:26259 --background

./cockroach start --insecure --store=/tmp/crdb-node2 \
  --listen-addr=localhost:26258 --http-addr=localhost:8182 \
  --join=localhost:26257,localhost:26258,localhost:26259 --background

./cockroach start --insecure --store=/tmp/crdb-node3 \
  --listen-addr=localhost:26259 --http-addr=localhost:8183 \
  --join=localhost:26257,localhost:26258,localhost:26259 --background

./cockroach init --insecure --host=localhost:26257
```

2. Set the enterprise license (use the `ENTERPRISE_LIC` value from P2):

```bash
./cockroach sql --insecure -e \
  "SET CLUSTER SETTING enterprise.license = '<ENTERPRISE_LIC>';"
```

3. Query the virtual table:

```bash
./cockroach sql --insecure -e \
  "SET allow_unsafe_internals = true; SELECT * FROM crdb_internal.node_license_status;"
```

### Expected Output

| Column | Expected Value |
|--------|---------------|
| has_license | `t` (true) |
| license_type | `enterprise` |
| edition | `enterprise` |
| add_ons | `data-replication, advanced-compliance` |
| vcpu_entitled | `16` |
| vcpu_observed | system vCPU count |
| expiration | ~1 year from license generation (matches EXPIRY) |
| grace_period_end | `NULL` |
| requires_telemetry | `f` (false) |
| is_disabled | `f` (false) |

### Pass Criteria

- All new fields (`edition`, `add_ons`, `vcpu_entitled`) are correctly populated.
- `has_license` is `true`, `license_type` is `enterprise`.

**Do not tear down the cluster — it is reused by Test 2A.4.**

---

## Test 2A.4: License Change Mid-Run

**Purpose:** Verify that changing the license at runtime updates the virtual
table values immediately.

**Prerequisite:** The 3-node cluster from Test 2A.1 must still be running.

### Steps

1. Set the evaluation license (use the `EVAL_LIC` value from P2):

```bash
./cockroach sql --insecure -e \
  "SET CLUSTER SETTING enterprise.license = '<EVAL_LIC>';"
```

2. Query the virtual table:

```bash
./cockroach sql --insecure -e \
  "SET allow_unsafe_internals = true; SELECT * FROM crdb_internal.node_license_status;"
```

### Expected Output

| Column | Expected Value |
|--------|---------------|
| has_license | `t` (true) |
| license_type | `evaluation` |
| edition | `standard` |
| add_ons | `data-replication` |
| vcpu_entitled | `8` |
| vcpu_observed | system vCPU count |
| is_disabled | `f` (false) |

### Pass Criteria

- Values changed from the enterprise license to match the evaluation license.
- `edition` changed from `enterprise` to `standard`.
- `add_ons` changed from `data-replication, advanced-compliance` to `data-replication`.
- `vcpu_entitled` changed from `16` to `8`.

### Cleanup

```bash
pkill -f "cockroach start" 2>/dev/null; sleep 2
pkill -9 -f "cockroach start" 2>/dev/null
rm -rf /tmp/crdb-node1 /tmp/crdb-node2 /tmp/crdb-node3
```

---

## Test 2A.2: Telemetry Log Output

**Purpose:** Verify that `license_validation_event` entries in the telemetry
log contain all new license fields.

**Key behavior:**
- Telemetry emission happens inside the diagnostics reporting loop
  (`pkg/server/diagnostics/reporter.go:206`).
- The default `diagnostics.reporting.interval` is 1 hour.
- Changing the interval mid-cycle does NOT reset the current timer; it only
  takes effect on the next cycle.
- Single-node clusters have `is_disabled=true`, which skips telemetry emission.
- Therefore we use a 3-node cluster and set the license + short interval
  BEFORE restarting, so the settings are loaded on boot.

### Steps

1. Start a fresh 3-node cluster, initialize, set license and interval:

```bash
./cockroach start --insecure --store=/tmp/crdb-node1 \
  --listen-addr=localhost:26257 --http-addr=localhost:8181 \
  --join=localhost:26257,localhost:26258,localhost:26259 --background

./cockroach start --insecure --store=/tmp/crdb-node2 \
  --listen-addr=localhost:26258 --http-addr=localhost:8182 \
  --join=localhost:26257,localhost:26258,localhost:26259 --background

./cockroach start --insecure --store=/tmp/crdb-node3 \
  --listen-addr=localhost:26259 --http-addr=localhost:8183 \
  --join=localhost:26257,localhost:26258,localhost:26259 --background

./cockroach init --insecure --host=localhost:26257
```

2. Set the license and reporting interval:

```bash
./cockroach sql --insecure -e \
  "SET CLUSTER SETTING enterprise.license = '<ENTERPRISE_LIC>';"

./cockroach sql --insecure -e \
  "SET CLUSTER SETTING diagnostics.reporting.interval = '10s';"
```

3. **Restart all nodes** so the settings take effect from boot:

```bash
pkill -f "cockroach start"; sleep 3
pkill -9 -f "cockroach start" 2>/dev/null; sleep 1

./cockroach start --insecure --store=/tmp/crdb-node1 \
  --listen-addr=localhost:26257 --http-addr=localhost:8181 \
  --join=localhost:26257,localhost:26258,localhost:26259 --background

./cockroach start --insecure --store=/tmp/crdb-node2 \
  --listen-addr=localhost:26258 --http-addr=localhost:8182 \
  --join=localhost:26257,localhost:26258,localhost:26259 --background

./cockroach start --insecure --store=/tmp/crdb-node3 \
  --listen-addr=localhost:26259 --http-addr=localhost:8183 \
  --join=localhost:26257,localhost:26258,localhost:26259 --background
```

4. Wait ~20 seconds for the first reporting cycle, then check the telemetry log:

```bash
sleep 20
grep "license_validation_event" /tmp/crdb-node1/logs/cockroach-telemetry*.log
```

### Expected Output

A JSON log entry like:

```json
{
  "EventType": "license_validation_event",
  "LicenseID": "<LICENSE_ID from P2>",
  "OrganizationID": "<ORG_ID from P2>",
  "Edition": "enterprise",
  "AddOns": ["data-replication", "advanced-compliance"],
  "ExpirationTimestamp": <EXPIRY from P2>,
  "VCPUEntitled": 16,
  "VCPUObserved": <system vCPU count>,
  "LicenseType": "enterprise"
}
```

### Pass Criteria

- A `license_validation_event` entry exists in `cockroach-telemetry.log`.
- The entry contains `LicenseID`, `OrganizationID`, `Edition`, `AddOns`,
  `VCPUEntitled` fields with values matching the generated license.
- `LicenseType` is `enterprise`.

### Troubleshooting

- If no telemetry entry appears after 20s, wait longer (up to 60s). The first
  emission happens on the first reporting cycle after boot; the default 1-hour
  timer is used if the persisted 10s setting hasn't been loaded yet.
- If the entry shows `"LicenseType":"none"`, it was emitted before the license
  was loaded. Wait for the next emission cycle.

**Do not tear down — the cluster is reused by Test 2A.5.**

---

## Test 2A.5: Debug Zip Contains Telemetry Logs

**Purpose:** Verify that `cockroach debug zip` captures the telemetry log files
and that those files contain `license_validation_event` entries with all
expected fields.

**Prerequisite:** The 3-node cluster from Test 2A.2 must still be running, with
telemetry logs already emitted.

### Steps

1. Generate the debug zip from node 1:

```bash
./cockroach debug zip /tmp/debug-test.zip --host=localhost:26257 --insecure
```

2. Extract the zip:

```bash
mkdir -p /tmp/debug-test-extracted
unzip -o /tmp/debug-test.zip -d /tmp/debug-test-extracted
```

3. Verify that telemetry log files are present in the zip:

```bash
find /tmp/debug-test-extracted -name "cockroach-telemetry*" -type f
```

4. Search the telemetry logs for `license_validation_event` entries:

```bash
grep -r "license_validation_event" /tmp/debug-test-extracted/
```

5. Verify the event contains the expected fields:

```bash
grep -r "license_validation_event" /tmp/debug-test-extracted/ | head -1 | \
  python3 -c "
import sys, json
line = sys.stdin.read().strip()
# Extract the JSON portion (after the structured log prefix)
idx = line.find('{')
if idx == -1:
    print('ERROR: No JSON found'); sys.exit(1)
obj = json.loads(line[idx:])
required = ['LicenseID', 'OrganizationID', 'Edition', 'AddOns',
            'VCPUEntitled', 'VCPUObserved', 'LicenseType']
missing = [f for f in required if f not in obj]
if missing:
    print(f'FAIL: Missing fields: {missing}')
    sys.exit(1)
print('PASS: All required fields present')
for f in required:
    print(f'  {f} = {obj[f]}')
"
```

### Expected Output

Step 3 should list one or more files matching `cockroach-telemetry*.log` under
`nodes/1/logs/`, `nodes/2/logs/`, and/or `nodes/3/logs/`.

Step 4 should show at least one line containing `license_validation_event`.

Step 5 should print:

```
PASS: All required fields present
  LicenseID = <LICENSE_ID from P2>
  OrganizationID = <ORG_ID from P2>
  Edition = enterprise
  AddOns = ['data-replication', 'advanced-compliance']
  VCPUEntitled = 16
  VCPUObserved = <system vCPU count>
  LicenseType = enterprise
```

### Pass Criteria

- Telemetry log files (`cockroach-telemetry*.log`) are present inside the
  debug zip under `nodes/<id>/logs/`.
- At least one `license_validation_event` entry exists in the extracted
  telemetry logs.
- The event contains all required fields: `LicenseID`, `OrganizationID`,
  `Edition`, `AddOns`, `VCPUEntitled`, `VCPUObserved`, `LicenseType`.
- Field values match the license generated in P2.

### Cleanup

```bash
pkill -f "cockroach start" 2>/dev/null; sleep 2
pkill -9 -f "cockroach start" 2>/dev/null
rm -rf /tmp/crdb-node1 /tmp/crdb-node2 /tmp/crdb-node3
rm -rf /tmp/debug-test.zip /tmp/debug-test-extracted
```

---

## Execution Notes for Claude

### Test Order

Tests should be run in this order:
1. **P1** (build) — only if binary hasn't been built recently
2. **P2** (generate licenses)
3. **P3** (cleanup)
4. **Test 2A.3** (no license, standalone)
5. **Test 2A.1** (install license, starts 3-node cluster)
6. **Test 2A.4** (license change, reuses 2A.1 cluster)
7. **Test 2A.2** (telemetry log, fresh 3-node cluster)
8. **Test 2A.5** (debug zip verification, reuses 2A.2 cluster)

### Port Conflicts

- CockroachDB SQL: 26257, 26258, 26259
- CockroachDB HTTP: 8181, 8182, 8183

If any port is already in use, kill the conflicting process first. Common
conflict: an old `cockroach` process on port 26257. Use
`lsof -i :26257 | grep LISTEN` to find and kill it.

### Common Issues

- **`crdb_internal.node_license_status` does not exist:** The running binary
  is an old version without the virtual table. Rebuild and restart.
- **`Access to crdb_internal and system is restricted`:** Prefix SQL queries
  with `SET allow_unsafe_internals = true;`.
- **Telemetry not emitted:** Single-node clusters set `is_disabled=true`. Use
  a 3-node cluster. Also, the diagnostics reporting interval change only takes
  effect after the current timer cycle expires — restart the cluster.
- **Node fails to start (lock file error):** A previous instance didn't shut
  down cleanly. Use `pkill -9 -f "cockroach start"` then `rm -rf /tmp/crdb-node*`.

### License Generation Package Note

The `licenseccl` package (`pkg/ccl/utilccl/licenseccl`) does not have its own
Bazel test target. The license generation test function must be added to
`pkg/ccl/utilccl/license_check_test.go` (package `utilccl`), which has the
necessary test infrastructure. The function references `licenseccl.License`,
`(*License).Encode()`, etc. via the import.

### Telemetry Log Location

Telemetry events are written to a separate log file (not the main log):
`<store-dir>/logs/cockroach-telemetry.<hostname>.<user>.<timestamp>.<pid>.log`

The filename includes the hostname, user, timestamp, and PID, so use a glob
pattern when grepping: `cockroach-telemetry*.log`

For node 1: `/tmp/crdb-node1/logs/cockroach-telemetry*.log`
