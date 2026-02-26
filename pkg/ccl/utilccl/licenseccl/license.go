// Copyright 2017 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package licenseccl

import (
	"encoding/base64"
	"strings"

	"github.com/cockroachdb/cockroach/pkg/util/protoutil"
	"github.com/cockroachdb/errors"
)

// LicensePrefix is a prefix on license strings to make them easily recognized.
const LicensePrefix = "crl-0-"

// Encode serializes the License as a base64 string.
func (l *License) Encode() (string, error) {
	bytes, err := protoutil.Marshal(l)
	if err != nil {
		return "", err
	}
	return LicensePrefix + base64.RawStdEncoding.EncodeToString(bytes), nil
}

// Decode attempts to read a base64 encoded License.
func Decode(s string) (*License, error) {
	if s == "" {
		return nil, nil
	}
	if !strings.HasPrefix(s, LicensePrefix) {
		return nil, errors.New("invalid license string")
	}
	s = strings.TrimPrefix(s, LicensePrefix)
	data, err := base64.RawStdEncoding.DecodeString(s)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid license string")
	}
	var lic License
	if err := protoutil.Unmarshal(data, &lic); err != nil {
		return nil, errors.Wrap(err, "invalid license string")
	}
	return &lic, nil
}

func (u License_Environment) String() string {
	switch u {
	case Unspecified:
		return ""
	case Production:
		return "production"
	case PreProduction:
		return "pre-production"
	case Development:
		return "development"
	default:
		return "other"
	}
}

func (e License_Edition) String() string {
	switch e {
	case EDITION_UNSPECIFIED:
		return ""
	case STANDARD:
		return "standard"
	case ENTERPRISE_EDITION:
		return "enterprise"
	case MISSION_CRITICAL:
		return "mission-critical"
	default:
		return "other"
	}
}

func (a License_AddOn) String() string {
	switch a {
	case ADD_ON_UNSPECIFIED:
		return ""
	case DATA_REPLICATION:
		return "data-replication"
	case ADVANCED_WORKLOAD_MGMT:
		return "advanced-workload-mgmt"
	case DATA_SYNCHRONIZATION:
		return "data-synchronization"
	case ADVANCED_COMPLIANCE:
		return "advanced-compliance"
	default:
		return "other"
	}
}
