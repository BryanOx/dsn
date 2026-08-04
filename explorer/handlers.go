package explorer

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/BryanOx/dsn/indexer"
	"github.com/BryanOx/dsn/rpc/service"
	"github.com/BryanOx/dsn/types"
	"github.com/gorilla/mux"
)

// Error codes
const (
	ErrCodeNotFound      = 404
	ErrCodeBadRequest    = 400
	ErrCodeInternalError = 500
	ErrCodeIndexerNA     = 503
)

// Handler functions

// handleBlocks returns paginated block list.
func handleBlocks(idx *indexer.Indexer, svc service.NodeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse pagination params
		offset, limit := parsePagination(r)

		// Check if indexer is available
		if idx == nil || !idx.IsEnabled() {
			WriteError(w, http.StatusServiceUnavailable, "indexer not available", ErrCodeIndexerNA)
			return
		}

		// Get latest height
		latestHeight := idx.GetLatestHeight()
		if latestHeight == 0 {
			WriteJSON(w, http.StatusOK, PaginatedResponse{
				Data:   []BlockSummary{},
				Offset: offset,
				Limit:  limit,
			})
			return
		}

		// Calculate total count
		totalCount := latestHeight + 1

		// Calculate start and end block numbers (newest first)
		startBlock := uint64(0)
		if latestHeight >= offset {
			startBlock = latestHeight - offset
		}
		endBlock := uint64(0)
		if startBlock >= limit {
			endBlock = startBlock - limit + 1
		} else {
			endBlock = 0
		}

		// Fetch blocks
		blocks := make([]BlockSummary, 0)
		for height := startBlock; height >= endBlock && height < latestHeight+1; height-- {
			blockData, err := idx.GetBlock(height)
			if err != nil || blockData == nil {
				continue
			}

			// Get block hash - for now use a placeholder
			blockHash := fmt.Sprintf("0x%032x", height)

			blocks = append(blocks, BlockSummary{
				Number:    height,
				Hash:      blockHash,
				Timestamp: blockData.Header.Timestamp,
				TxCount:   blockData.TxCount,
			})
		}

		WriteJSON(w, http.StatusOK, PaginatedResponse{
			Data:       blocks,
			Offset:     offset,
			Limit:      limit,
			TotalCount: totalCount,
		})
	}
}

// handleBlockDetail returns full block data with transactions.
func handleBlockDetail(idx *indexer.Indexer, svc service.NodeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["numberOrHash"]

		// Check if indexer is available
		if idx == nil || !idx.IsEnabled() {
			WriteError(w, http.StatusServiceUnavailable, "indexer not available", ErrCodeIndexerNA)
			return
		}

		// Parse block number or hash
		var height uint64
		var err error

		// Try as number first
		if strings.HasPrefix(id, "0x") {
			// It's a hash - for now, we can't look up by hash without more indexer methods
			// Try parsing as number
			height, err = strconv.ParseUint(id, 0, 64)
			if err != nil {
				WriteError(w, http.StatusBadRequest, "invalid block identifier", ErrCodeBadRequest)
				return
			}
		} else {
			height, err = strconv.ParseUint(id, 10, 64)
			if err != nil {
				WriteError(w, http.StatusBadRequest, "invalid block number", ErrCodeBadRequest)
				return
			}
		}

		// Get block data
		blockData, err := idx.GetBlock(height)
		if err != nil || blockData == nil {
			WriteError(w, http.StatusNotFound, "block not found", ErrCodeNotFound)
			return
		}

		// Get transactions for this block
		txs, _ := idx.GetTransactionsByBlock(height)

		// Build transaction items
		txItems := make([]TransactionItem, 0, len(txs))
		for _, tx := range txs {
			status := uint8(1)
			if !tx.Status {
				status = 0
			}
			txItems = append(txItems, TransactionItem{
				Hash:     hashToHex(tx.Hash),
				Sender:   tx.Data.Sender.String(),
				Nonce:    tx.Data.Nonce,
				Status:   status,
				GasUsed:  tx.GasUsed,
				BlockNum: tx.BlockNumber,
			})
		}

		// Get parent hash
		parentHash := "0x"
		if blockData.Header.PreviousHash != (types.Hash{}) {
			parentHash = hashToHex(blockData.Header.PreviousHash)
		}

		// Get state root
		stateRoot := "0x"
		if blockData.Header.StateRoot != (types.Hash{}) {
			stateRoot = hashToHex(blockData.Header.StateRoot)
		}

		// Build block hash
		blockHash := fmt.Sprintf("0x%032x", height)

		detail := BlockDetail{
			BlockSummary: BlockSummary{
				Number:    height,
				Hash:      blockHash,
				Timestamp: blockData.Header.Timestamp,
				TxCount:   blockData.TxCount,
			},
			ParentHash:   parentHash,
			StateRoot:    stateRoot,
			Transactions: txItems,
		}

		WriteJSON(w, http.StatusOK, detail)
	}
}

// handleTransaction returns transaction details with receipt and events.
func handleTransaction(idx *indexer.Indexer, svc service.NodeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		hashStr := vars["hash"]

		// Check if indexer is available
		if idx == nil || !idx.IsEnabled() {
			WriteError(w, http.StatusServiceUnavailable, "indexer not available", ErrCodeIndexerNA)
			return
		}

		// Parse hash - remove 0x prefix if present
		hashStr = strings.TrimPrefix(hashStr, "0x")
		hashBytes, err := hex.DecodeString(hashStr)
		if err != nil || len(hashBytes) != 32 {
			WriteError(w, http.StatusBadRequest, "invalid transaction hash", ErrCodeBadRequest)
			return
		}

		var hash types.Hash
		copy(hash[:], hashBytes)

		// Get transaction from indexer
		receipt, err := idx.GetTransaction(hash)
		if err != nil || receipt == nil {
			WriteError(w, http.StatusNotFound, "transaction not found", ErrCodeNotFound)
			return
		}

		// Build transaction item
		status := uint8(1)
		if !receipt.Status {
			status = 0
		}

		txItem := TransactionItem{
			Hash:     hashToHex(receipt.Hash),
			Sender:   receipt.Data.Sender.String(),
			Nonce:    receipt.Data.Nonce,
			Status:   status,
			GasUsed:  receipt.GasUsed,
			BlockNum: receipt.BlockNumber,
		}

		WriteJSON(w, http.StatusOK, txItem)
	}
}

// handleAccount returns account information.
func handleAccount(idx *indexer.Indexer, svc service.NodeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		address := vars["address"]

		// Validate address format
		addr, err := types.ParseAddress(address)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid address format", ErrCodeBadRequest)
			return
		}

		// Get account from service
		accResult, err := svc.GetAccount(r.Context(), addr.String())
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to get account", ErrCodeInternalError)
			return
		}

		// Check if it's a contract (has non-empty code hash)
		isContract := accResult.CodeHash != "" && accResult.CodeHash != "0x"

		// Get recent transactions from indexer if available
		recentTxs := []string{}
		if idx != nil && idx.IsEnabled() {
			// Could implement GetTransactionsByAddress in indexer
			// For now, leave empty
		}

		info := AccountInfo{
			Address:    address,
			Balance:    accResult.Balance,
			Nonce:      accResult.Nonce,
			IsContract: isContract,
			RecentTxs:  recentTxs,
		}

		WriteJSON(w, http.StatusOK, info)
	}
}

// handleContract returns contract metadata and recent events.
func handleContract(idx *indexer.Indexer, svc service.NodeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		address := vars["address"]

		// Validate address
		addr, err := types.ParseAddress(address)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid address format", ErrCodeBadRequest)
			return
		}

		// Get contract from service
		contractResult, err := svc.GetContract(r.Context(), addr.String())
		if err != nil {
			if err == service.ErrNotFound {
				WriteError(w, http.StatusNotFound, "contract not found", ErrCodeNotFound)
				return
			}
			WriteError(w, http.StatusInternalServerError, "failed to get contract", ErrCodeInternalError)
			return
		}

		// Build contract info
		info := ContractInfo{
			Address:  address,
			CodeHash: contractResult.CodeHash,
			Metadata: nil,
		}

		// Add metadata if available
		if contractResult.Metadata != nil {
			info.Metadata = &ContractMeta{
				Name:       contractResult.Metadata.Name,
				Version:    contractResult.Metadata.Version,
				Entrypoint: contractResult.Metadata.Entrypoint,
			}
		}

		// Get recent events if indexer available
		if idx != nil && idx.IsEnabled() {
			// Convert address bytes to a types.Hash for the filter
			var contractHash types.Hash
			copy(contractHash[:], addr[:])
			filter := indexer.EventFilter{
				Contract: &contractHash,
			}
			events, err := idx.GetEvents(filter)
			if err == nil && len(events) > 0 {
				// Limit to recent 10 events
				recentEvents := make([]EventItem, 0, len(events))
				if len(events) > 10 {
					events = events[:10]
				}
				for _, e := range events {
					recentEvents = append(recentEvents, EventItem{
						Contract:    hashToHex(e.Event.ContractID),
						Topics:      []string{e.Event.Topic},
						Data:        bytesToHex(e.Event.Data),
						BlockNumber: e.BlockNumber,
					})
				}
				info.RecentEvents = recentEvents
			}
		}

		WriteJSON(w, http.StatusOK, info)
	}
}

// handleValidators returns validator list.
func handleValidators(svc service.NodeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse optional epoch parameter
		epochStr := r.URL.Query().Get("epoch")
		var epochPtr *uint64
		if epochStr != "" {
			epoch, err := strconv.ParseUint(epochStr, 10, 64)
			if err != nil {
				WriteError(w, http.StatusBadRequest, "invalid epoch parameter", ErrCodeBadRequest)
				return
			}
			epochPtr = &epoch
		}

		// Get validators from service
		validators, err := svc.GetValidators(r.Context(), epochPtr)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to get validators", ErrCodeInternalError)
			return
		}

		// Convert to explorer format
		result := make([]ValidatorInfo, len(validators))
		for i, v := range validators {
			result[i] = ValidatorInfo{
				Address:    v.Address,
				Power:      v.VotingPower,
				Commission: v.Commission,
			}
		}

		WriteJSON(w, http.StatusOK, result)
	}
}

// handleEvents returns filtered events.
func handleEvents(idx *indexer.Indexer, svc service.NodeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if indexer is available
		if idx == nil || !idx.IsEnabled() {
			WriteError(w, http.StatusServiceUnavailable, "indexer not available", ErrCodeIndexerNA)
			return
		}

		// Parse query params
		offset, limit := parsePagination(r)

		filter := indexer.EventFilter{}

		// Contract filter
		contractStr := r.URL.Query().Get("contract")
		if contractStr != "" {
			addr, err := types.ParseAddress(contractStr)
			if err == nil {
				// Convert address bytes to a types.Hash for the filter
				var contractHash types.Hash
				copy(contractHash[:], addr[:])
				filter.Contract = &contractHash
			}
		}

		// Topic0 filter
		filter.Topic0 = r.URL.Query().Get("topic0")

		// FromBlock filter
		fromBlockStr := r.URL.Query().Get("fromBlock")
		if fromBlockStr != "" {
			fromBlock, err := strconv.ParseUint(fromBlockStr, 10, 64)
			if err == nil {
				filter.FromBlock = fromBlock
			}
		}

		// ToBlock filter
		toBlockStr := r.URL.Query().Get("toBlock")
		if toBlockStr != "" {
			toBlock, err := strconv.ParseUint(toBlockStr, 10, 64)
			if err == nil {
				filter.ToBlock = toBlock
			}
		}

		// Get events from indexer
		events, err := idx.GetEvents(filter)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to get events", ErrCodeInternalError)
			return
		}

		// Apply pagination
		totalCount := uint64(len(events))
		if offset > totalCount {
			offset = totalCount
		}
		endIdx := offset + limit
		if endIdx > totalCount {
			endIdx = totalCount
		}

		paginatedEvents := events[offset:endIdx]

		// Convert to explorer format
		result := make([]EventItem, len(paginatedEvents))
		for i, e := range paginatedEvents {
			result[i] = EventItem{
				Contract:    hashToHex(e.Event.ContractID),
				Topics:      []string{e.Event.Topic},
				Data:        bytesToHex(e.Event.Data),
				BlockNumber: e.BlockNumber,
			}
		}

		WriteJSON(w, http.StatusOK, PaginatedResponse{
			Data:       result,
			Offset:     offset,
			Limit:      limit,
			TotalCount: totalCount,
		})
	}
}

// handleSupply returns token supply metrics.
func handleSupply(svc service.NodeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get supply from service
		supply, err := svc.GetSupply(r.Context())
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to get supply", ErrCodeInternalError)
			return
		}

		info := SupplyInfo{
			Total:       supply.Total,
			Circulating: supply.Circulating,
			Staked:      supply.Staked,
		}

		WriteJSON(w, http.StatusOK, info)
	}
}

// Helper functions

func parsePagination(r *http.Request) (offset, limit uint64) {
	offsetStr := r.URL.Query().Get("offset")
	limitStr := r.URL.Query().Get("limit")

	offset = 0
	limit = 20 // default

	if offsetStr != "" {
		if o, err := strconv.ParseUint(offsetStr, 10, 64); err == nil {
			offset = o
		}
	}

	if limitStr != "" {
		if l, err := strconv.ParseUint(limitStr, 10, 64); err == nil {
			limit = l
		}
	}

	// Cap limit at 100
	if limit > 100 {
		limit = 100
	}

	return
}

func bytesToHex(b []byte) string {
	if len(b) == 0 {
		return "0x"
	}
	return "0x" + hex.EncodeToString(b)
}
