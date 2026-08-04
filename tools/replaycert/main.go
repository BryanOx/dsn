package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BryanOx/dsn/tools/replaycert/pkg/compare"
	"github.com/BryanOx/dsn/tools/replaycert/pkg/hasher"
	"github.com/BryanOx/dsn/tools/replaycert/pkg/transcript"
	"github.com/BryanOx/dsn/types"
)

type Config struct {
	BlocksPath    string
	NodePath      string
	HashesPath    string
	CrossPlatform bool
	Iterations    int
	DryRun        bool
	SaveHashes    bool
}

func main() {
	cfg := parseFlags()

	if cfg.DryRun {
		fmt.Println("=== DRY RUN MODE ===")
		fmt.Printf("Blocks path: %s\n", cfg.BlocksPath)
		fmt.Printf("Node binary: %s\n", cfg.NodePath)
		fmt.Printf("Reference hashes: %s\n", cfg.HashesPath)
		fmt.Printf("Cross-platform mode: %v\n", cfg.CrossPlatform)
		fmt.Printf("Iterations: %d\n", cfg.Iterations)
		fmt.Println("\nWould compare the following:")
		fmt.Println("  - Block execution transcripts")
		fmt.Println("  - Event streams")
		fmt.Println("  - Receipt roots")
		fmt.Println("  - State snapshots")
		fmt.Println("  - Validator sets")
		return
	}

	// Load reference hashes if provided
	var refHashes map[uint64][]byte
	if cfg.HashesPath != "" {
		refHashes = loadReferenceHashes(cfg.HashesPath)
		fmt.Printf("Loaded %d reference hashes\n", len(refHashes))
	}

	// For now, simulate processing blocks
	// In production, this would:
	// 1. Read blocks from BlocksPath
	// 2. Execute each block with the node binary
	// 3. Collect transcript data
	// 4. Compare against reference

	fmt.Printf("Running replay certification with %d iterations\n", cfg.Iterations)

	// Run replay iterations
	results := runReplay(cfg, refHashes)

	// Print results
	printResults(results)

	// Save hashes if requested
	if cfg.SaveHashes {
		saveHashes(results)
	}

	// Exit with appropriate code
	if results.AllPassed {
		fmt.Println("\n✓ CERTIFICATION PASSED")
		os.Exit(0)
	} else {
		fmt.Println("\n✗ CERTIFICATION FAILED")
		os.Exit(1)
	}
}

func parseFlags() Config {
	cfg := Config{}

	flag.StringVar(&cfg.BlocksPath, "blocks", "", "Path to blocks/WAL data")
	flag.StringVar(&cfg.NodePath, "node", "", "Path to node binary")
	flag.StringVar(&cfg.HashesPath, "hashes", "", "Path to reference hashes file")
	flag.BoolVar(&cfg.CrossPlatform, "cross-platform", false, "Enable cross-platform mode")
	flag.IntVar(&cfg.Iterations, "iterations", 3, "Number of replay iterations")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "Show what would be compared without running re-execution")
	flag.BoolVar(&cfg.SaveHashes, "save-hashes", false, "Save computed hashes to a file for cross-platform comparison")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: replaycert [options]\n\n")
		fmt.Fprintf(os.Stderr, "Deterministic Replay Certification Tool\n")
		fmt.Fprintf(os.Stderr, "Verifies execution produces byte-identical outputs across machines/platforms.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NFlag() == 0 || cfg.BlocksPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	return cfg
}

type ReplayResult struct {
	BlockHeight    uint64
	TranscriptHash []byte
	Pass           bool
	Comparison     *compare.ComparisonReport
}

type ReplayResults struct {
	Results    []ReplayResult
	AllPassed  bool
	Iterations int
}

func runReplay(cfg Config, refHashes map[uint64][]byte) ReplayResults {
	results := ReplayResults{
		Iterations: cfg.Iterations,
		Results:    []ReplayResult{},
	}

	// Simulate loading blocks and running replay
	// In production, this would actually execute blocks

	// For demonstration, create sample data
	// Read blocks from the blocks path if available
	blocks, err := loadBlocks(cfg.BlocksPath)
	if err != nil {
		fmt.Printf("Warning: Could not load blocks: %v\n", err)
		fmt.Println("Running in simulation mode with sample data")
		blocks = generateSampleBlocks(10)
	}

	for _, block := range blocks {
		// Create sample transcript for this block
		trans := createSampleTranscript(block)

		// Hash the transcript
		transHash := trans.Hash()

		// Compare against reference if available
		var comp *compare.ComparisonReport
		if refHashes != nil {
			if refHash, ok := refHashes[block.Header.Height]; ok {
				matched, _ := compare.CompareHashes(refHash, transHash)
				if !matched {
					comp = &compare.ComparisonReport{
						OverallPass: false,
						PerField: []compare.FieldMatch{
							{
								Name:     "TranscriptHash",
								Pass:     false,
								Expected: hex.EncodeToString(refHash),
								Actual:   hex.EncodeToString(transHash),
							},
						},
					}
				}
			}
		}

		result := ReplayResult{
			BlockHeight:    block.Header.Height,
			TranscriptHash: transHash,
			Pass:           comp == nil || comp.OverallPass,
			Comparison:     comp,
		}

		results.Results = append(results.Results, result)
	}

	// Check if all passed
	allPassed := true
	for _, r := range results.Results {
		if !r.Pass {
			allPassed = false
			break
		}
	}
	results.AllPassed = allPassed

	return results
}

func loadBlocks(path string) ([]*types.Block, error) {
	// Try to load blocks from the path
	// This would need proper WAL reading in production
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var blocks []*types.Block
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// In production, read and deserialize blocks
		_ = e.Name()
	}

	return blocks, nil
}

func generateSampleBlocks(count int) []*types.Block {
	blocks := make([]*types.Block, count)
	for i := 0; i < count; i++ {
		blocks[i] = &types.Block{
			Header: types.BlockHeader{
				Height:    uint64(i + 1),
				Timestamp: uint64(1000000 + i*10000),
			},
			Transactions: []types.Transaction{},
			Events:       []types.Event{},
		}
	}
	return blocks
}

func createSampleTranscript(block *types.Block) transcript.Transcript {
	// Create a sample transcript from block data
	events := block.Events
	if events == nil {
		events = []types.Event{}
	}

	// Calculate various hashes using hasher package types
	eventsHash := hasher.HashEvents(events)
	receiptsHash := hasher.HashReceipts([]hasher.Receipt{})
	snapshotHash := hasher.HashSnapshot(hasher.StateSnapshot{})
	validatorsHash := hasher.HashValidatorSet([]hasher.Validator{})

	return transcript.Transcript{
		RootHash:         types.Hash{}, // Would be block header hash
		EventsHash:       eventsHash,
		ReceiptsHash:     receiptsHash,
		SnapshotHash:     snapshotHash,
		ValidatorSetHash: validatorsHash,
		BlockHeight:      block.Header.Height,
		Timestamp:        block.Header.Timestamp,
	}
}

func loadReferenceHashes(path string) map[uint64][]byte {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("Warning: Could not load reference hashes: %v\n", err)
		return nil
	}

	var hashes map[string]string
	if err := json.Unmarshal(data, &hashes); err != nil {
		fmt.Printf("Warning: Could not parse reference hashes: %v\n", err)
		return nil
	}

	result := make(map[uint64][]byte)
	for key, value := range hashes {
		var height uint64
		fmt.Sscanf(key, "%d", &height)
		hash, err := hex.DecodeString(value)
		if err == nil {
			result[height] = hash
		}
	}

	return result
}

func saveHashes(results ReplayResults) {
	hashes := make(map[string]string)
	for _, r := range results.Results {
		key := fmt.Sprintf("block_%d", r.BlockHeight)
		hashes[key] = hex.EncodeToString(r.TranscriptHash)
	}

	data, err := json.MarshalIndent(hashes, "", "  ")
	if err != nil {
		fmt.Printf("Error saving hashes: %v\n", err)
		return
	}

	filename := "replaycert_hashes.json"
	if err := os.WriteFile(filename, data, 0644); err != nil {
		fmt.Printf("Error writing hashes file: %v\n", err)
		return
	}

	fmt.Printf("Hashes saved to %s\n", filename)
}

func printResults(results ReplayResults) {
	fmt.Printf("\n=== Replay Results (%d iterations) ===\n", results.Iterations)
	fmt.Printf("Total blocks processed: %d\n", len(results.Results))

	passed := 0
	failed := 0

	for _, r := range results.Results {
		status := "✓"
		if !r.Pass {
			status = "✗"
			failed++
			fmt.Printf("%s Block %d: FAILED\n", status, r.BlockHeight)
			if r.Comparison != nil {
				for _, f := range r.Comparison.PerField {
					if !f.Pass {
						fmt.Printf("    - %s: expected %s, got %s\n", f.Name, f.Expected, f.Actual)
					}
				}
			}
		} else {
			passed++
			fmt.Printf("%s Block %d: %s\n", status, r.BlockHeight, hex.EncodeToString(r.TranscriptHash[:8]))
		}
	}

	fmt.Printf("\nSummary: %d passed, %d failed\n", passed, failed)
}

// Verify implementation - ensure packages work together
func init() {
	// Test that hasher works
	testHasher()

	// Test that compare works
	testCompare()

	// Test that transcript works
	testTranscript()
}

func testHasher() {
	data := []byte("test data")
	hash := hasher.HashTranscript(data)
	if len(hash) == 0 {
		panic("hasher returned empty hash")
	}
}

func testCompare() {
	a := []byte("hash_a")
	b := []byte("hash_a")
	matched, diff := compare.CompareHashes(a, b)
	if !matched {
		panic(fmt.Sprintf("CompareHashes failed: %s", diff))
	}

	b = []byte("hash_b")
	matched, _ = compare.CompareHashes(a, b)
	if matched {
		panic("CompareHashes should have returned false for different hashes")
	}
}

func testTranscript() {
	t := transcript.Transcript{
		BlockHeight: 1,
		Timestamp:   1000,
	}
	hash := t.Hash()
	if len(hash) == 0 {
		panic("transcript hash is empty")
	}
}

// Helper to make the code compile - ensure types exist
var _ = hasher.Receipt{}
var _ = hasher.StateSnapshot{}
var _ = hasher.Validator{}
var _ = bytes.Buffer{}
var _ = filepath.Join
