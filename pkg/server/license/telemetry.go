// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package license

import (
	"context"
	"encoding/hex"
	"runtime"

	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/cockroachdb/cockroach/pkg/util/log/eventpb"
	"github.com/cockroachdb/cockroach/pkg/util/log/severity"
	"github.com/cockroachdb/cockroach/pkg/util/timeutil"
)

// EmitLicenseValidationTelemetry emits a LicenseValidationEvent to the
// TELEMETRY log channel. This is intended to be called periodically (hourly)
// to record license state and cluster resource usage for audit purposes.
func (e *Enforcer) EmitLicenseValidationTelemetry(ctx context.Context) {
	if e.isDisabled.Load() {
		return
	}

	edInfo := e.GetEditionInfo()
	if edInfo == nil {
		return
	}

	// Observe vCPU count: use Go runtime's NumCPU which respects cgroup limits
	// on Linux with Go 1.19+.
	vcpuObserved := runtime.NumCPU()

	// Format edition string from the cached integer value.
	editionStr := formatEdition(edInfo.Edition)

	// Format add-on strings.
	addOnStrs := make([]string, 0, len(edInfo.AddOns))
	for _, a := range edInfo.AddOns {
		s := formatAddOn(a)
		if s != "" {
			addOnStrs = append(addOnStrs, s)
		}
	}

	event := &eventpb.LicenseValidationEvent{
		LicenseID:           FormatUUIDBytes(edInfo.LicenseID),
		OrganizationID:      FormatUUIDBytes(edInfo.OrganizationID),
		Edition:             editionStr,
		AddOns:              addOnStrs,
		ExpirationTimestamp: e.licenseExpiryTS.Load(),
		VCPUEntitled:        edInfo.VCPUEntitled,
		VCPUObserved:        int32(vcpuObserved),
		LicenseType:         e.GetCachedLicType().String(),
		CPUUtilizationPct:   getCPUUtilization(),
	}

	log.StructuredEvent(ctx, severity.INFO, event)
	log.Dev.Infof(ctx, "emitted license validation telemetry: edition=%s, vcpu_entitled=%d, vcpu_observed=%d, license_type=%s",
		editionStr, edInfo.VCPUEntitled, vcpuObserved, e.GetCachedLicType().String())
}

// getCPUUtilization returns the current CPU utilization as a percentage.
// For Q1 audit-only, this returns 0 as a placeholder. A future implementation
// can use runtime/metrics or cgroups to provide accurate values.
func getCPUUtilization() float64 {
	return 0
}

// formatEdition maps the edition integer to a human-readable string.
// These values mirror the License_Edition enum from licenseccl.
func formatEdition(edition int32) string {
	switch edition {
	case 0:
		return ""
	case 1:
		return "standard"
	case 2:
		return "enterprise"
	case 3:
		return "mission-critical"
	default:
		return "unknown"
	}
}

// formatAddOn maps the add-on integer to a human-readable string.
// These values mirror the License_AddOn enum from licenseccl.
func formatAddOn(addOn int32) string {
	switch addOn {
	case 0:
		return ""
	case 1:
		return "data-replication"
	case 2:
		return "advanced-workload-mgmt"
	case 3:
		return "data-synchronization"
	case 4:
		return "advanced-compliance"
	default:
		return "unknown"
	}
}

// FormatEdition is the exported version of formatEdition for use by
// crdb_internal virtual tables.
func FormatEdition(edition int32) string {
	return formatEdition(edition)
}

// FormatAddOns converts a slice of add-on integer values to a comma-separated
// human-readable string. Exported for use by crdb_internal virtual tables.
func FormatAddOns(addOns []int32) string {
	if len(addOns) == 0 {
		return ""
	}
	result := ""
	for i, a := range addOns {
		s := formatAddOn(a)
		if s == "" {
			continue
		}
		if i > 0 && result != "" {
			result += ", "
		}
		result += s
	}
	return result
}

// FormatUUIDBytes converts a UUID stored as raw bytes into hex string.
func FormatUUIDBytes(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return hex.EncodeToString(b)
}

// GetTelemetryTimestamp returns the current timestamp, useful for testing.
func GetTelemetryTimestamp() int64 {
	return timeutil.Now().Unix()
}
