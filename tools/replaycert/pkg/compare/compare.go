// Package compare provides comparison and diff reporting for deterministic replay certification.
package compare

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/dsn/dsn/tools/replaycert/pkg/transcript"
)

// CompareHashes compares two hash byte slices and returns whether they match,
// along with a description of any differences.
func CompareHashes(a, b []byte) (bool, string) {
	if len(a) != len(b) {
		return false, fmt.Sprintf("length mismatch: %d vs %d", len(a), len(b))
	}

	if !bytes.Equal(a, b) {
		return false, fmt.Sprintf("hash mismatch: %s vs %s", hex.EncodeToString(a), hex.EncodeToString(b))
	}

	return true, "hashes match"
}

// CompareTranscripts compares an expected transcript against an actual transcript
// and returns a detailed comparison report.
func CompareTranscripts(expected, actual transcript.Transcript) (*ComparisonReport, error) {
	report := &ComparisonReport{
		OverallPass: true,
		PerField:    []FieldMatch{},
	}

	// Compare RootHash
	if !bytes.Equal(expected.RootHash[:], actual.RootHash[:]) {
		report.OverallPass = false
		report.PerField = append(report.PerField, FieldMatch{
			Name:     "RootHash",
			Pass:     false,
			Expected: hex.EncodeToString(expected.RootHash[:]),
			Actual:   hex.EncodeToString(actual.RootHash[:]),
		})
	} else {
		report.PerField = append(report.PerField, FieldMatch{
			Name: "RootHash",
			Pass: true,
		})
	}

	// Compare EventsHash
	if !bytes.Equal(expected.EventsHash, actual.EventsHash) {
		report.OverallPass = false
		report.PerField = append(report.PerField, FieldMatch{
			Name:     "EventsHash",
			Pass:     false,
			Expected: hex.EncodeToString(expected.EventsHash),
			Actual:   hex.EncodeToString(actual.EventsHash),
		})
	} else {
		report.PerField = append(report.PerField, FieldMatch{
			Name: "EventsHash",
			Pass: true,
		})
	}

	// Compare ReceiptsHash
	if !bytes.Equal(expected.ReceiptsHash, actual.ReceiptsHash) {
		report.OverallPass = false
		report.PerField = append(report.PerField, FieldMatch{
			Name:     "ReceiptsHash",
			Pass:     false,
			Expected: hex.EncodeToString(expected.ReceiptsHash),
			Actual:   hex.EncodeToString(actual.ReceiptsHash),
		})
	} else {
		report.PerField = append(report.PerField, FieldMatch{
			Name: "ReceiptsHash",
			Pass: true,
		})
	}

	// Compare SnapshotHash
	if !bytes.Equal(expected.SnapshotHash, actual.SnapshotHash) {
		report.OverallPass = false
		report.PerField = append(report.PerField, FieldMatch{
			Name:     "SnapshotHash",
			Pass:     false,
			Expected: hex.EncodeToString(expected.SnapshotHash),
			Actual:   hex.EncodeToString(actual.SnapshotHash),
		})
	} else {
		report.PerField = append(report.PerField, FieldMatch{
			Name: "SnapshotHash",
			Pass: true,
		})
	}

	// Compare ValidatorSetHash
	if !bytes.Equal(expected.ValidatorSetHash, actual.ValidatorSetHash) {
		report.OverallPass = false
		report.PerField = append(report.PerField, FieldMatch{
			Name:     "ValidatorSetHash",
			Pass:     false,
			Expected: hex.EncodeToString(expected.ValidatorSetHash),
			Actual:   hex.EncodeToString(actual.ValidatorSetHash),
		})
	} else {
		report.PerField = append(report.PerField, FieldMatch{
			Name: "ValidatorSetHash",
			Pass: true,
		})
	}

	// Compare BlockHeight (should be identical)
	if expected.BlockHeight != actual.BlockHeight {
		report.OverallPass = false
		report.PerField = append(report.PerField, FieldMatch{
			Name:     "BlockHeight",
			Pass:     false,
			Expected: fmt.Sprintf("%d", expected.BlockHeight),
			Actual:   fmt.Sprintf("%d", actual.BlockHeight),
		})
	} else {
		report.PerField = append(report.PerField, FieldMatch{
			Name: "BlockHeight",
			Pass: true,
		})
	}

	// Compare Timestamp (should be identical for deterministic replay)
	if expected.Timestamp != actual.Timestamp {
		report.OverallPass = false
		report.PerField = append(report.PerField, FieldMatch{
			Name:     "Timestamp",
			Pass:     false,
			Expected: fmt.Sprintf("%d", expected.Timestamp),
			Actual:   fmt.Sprintf("%d", actual.Timestamp),
		})
	} else {
		report.PerField = append(report.PerField, FieldMatch{
			Name: "Timestamp",
			Pass: true,
		})
	}

	return report, nil
}

// ComparisonReport contains the results of comparing two transcripts.
type ComparisonReport struct {
	// OverallPass indicates if all fields matched
	OverallPass bool
	// PerField contains individual field comparison results
	PerField []FieldMatch
}

// FieldMatch represents the comparison result for a single field.
type FieldMatch struct {
	// Name is the field name
	Name string
	// Pass indicates if this field matched
	Pass bool
	// Expected contains the expected value (only populated if Pass is false)
	Expected string
	// Actual contains the actual value (only populated if Pass is false)
	Actual string
}

// String returns a human-readable representation of the comparison report.
func (r *ComparisonReport) String() string {
	if r.OverallPass {
		return "All fields match"
	}

	var buf bytes.Buffer
	buf.WriteString("Differences found:\n")
	for _, f := range r.PerField {
		if !f.Pass {
			buf.WriteString(fmt.Sprintf("  - %s: expected %s, got %s\n", f.Name, f.Expected, f.Actual))
		}
	}
	return buf.String()
}

// FieldNames returns all field names that would be compared.
func FieldNames() []string {
	return []string{
		"RootHash",
		"EventsHash",
		"ReceiptsHash",
		"SnapshotHash",
		"ValidatorSetHash",
		"BlockHeight",
		"Timestamp",
	}
}
