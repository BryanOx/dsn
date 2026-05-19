//go:build benchmark

package benchmarks

import (
	"fmt"
	"strings"
)

// BenchmarkReport holds benchmark results in a structured format.
type BenchmarkReport struct {
	Name        string
	N           int
	NSperOp     float64
	AllocsPerOp int64
	MBperSec    float64
}

// String returns a human-readable representation of the benchmark report.
func (r *BenchmarkReport) String() string {
	return fmt.Sprintf("%-50s %10d %12.2f ns/op %8d B/op %10.2f MB/s",
		r.Name, r.N, r.NSperOp, r.AllocsPerOp, r.MBperSec)
}

// CSV returns a CSV-formatted string of the benchmark report.
func (r *BenchmarkReport) CSV() string {
	return fmt.Sprintf("%s,%d,%.2f,%d,%.2f",
		r.Name, r.N, r.NSperOp, r.AllocsPerOp, r.MBperSec)
}

// CSVHeader returns the CSV header row.
func CSVHeader() string {
	return "Name,N,NSperOp,AllocsPerOp,MBperSec"
}

// ExportToMarkdown converts a slice of BenchmarkReports to a Markdown table.
func ExportToMarkdown(reports []BenchmarkReport) string {
	if len(reports) == 0 {
		return "No benchmark results to export."
	}

	var sb strings.Builder
	sb.WriteString("| Benchmark | N | ns/op | B/op | MB/s |\n")
	sb.WriteString("|-----------|-----|-------|------|------|\n")

	for _, r := range reports {
		sb.WriteString(fmt.Sprintf("| %s | %d | %.2f | %d | %.2f |\n",
			r.Name, r.N, r.NSperOp, r.AllocsPerOp, r.MBperSec))
	}

	return sb.String()
}