package sdk

import (
	"context"
	"math/big"
)

// GetBlock retrieves a block by number or hash.
func (c *Client) GetBlock(ctx context.Context, blockNumber *uint64, blockHash *string) (*Block, error) {
	params := GetBlockParams{
		BlockNumber: blockNumber,
		BlockHash:   blockHash,
	}

	var result Block
	if err := c.call(ctx, "dsn_getBlock", params, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetLatestBlock retrieves the latest block.
func (c *Client) GetLatestBlock(ctx context.Context) (*Block, error) {
	return c.GetBlock(ctx, nil, nil)
}

// GetTransaction retrieves a transaction by hash.
func (c *Client) GetTransaction(ctx context.Context, txHash string) (*Transaction, error) {
	params := GetTransactionParams{TxHash: txHash}

	var result Transaction
	if err := c.call(ctx, "dsn_getTransaction", params, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetAccount retrieves account data by address.
func (c *Client) GetAccount(ctx context.Context, address string) (*Account, error) {
	params := GetAccountParams{Address: address}

	var result Account
	if err := c.call(ctx, "dsn_getAccount", params, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetBalance retrieves account balance by address.
func (c *Client) GetBalance(ctx context.Context, address string) (*big.Int, error) {
	params := GetBalanceParams{Address: address}

	var result string
	if err := c.call(ctx, "dsn_getBalance", params, &result); err != nil {
		return nil, err
	}

	// Parse balance string to big.Int
	balance, ok := new(big.Int).SetString(result, 10)
	if !ok {
		return nil, ErrInternal
	}

	return balance, nil
}

// GetContract retrieves contract data by address.
func (c *Client) GetContract(ctx context.Context, address string) (*Contract, error) {
	params := GetContractParams{Address: address}

	var result Contract
	if err := c.call(ctx, "dsn_getContract", params, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// CallContract performs a local contract call.
func (c *Client) CallContract(ctx context.Context, address, data string, gasLimit uint64) (*CallResult, error) {
	params := CallContractParams{
		Address:  address,
		Data:     data,
		GasLimit: gasLimit,
	}

	var result CallResult
	if err := c.call(ctx, "dsn_callContract", params, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// EstimateGas estimates gas for a contract call.
func (c *Client) EstimateGas(ctx context.Context, address, data string) (uint64, error) {
	params := EstimateGasParams{
		Address: address,
		Data:    data,
	}

	var result EstimateResult
	if err := c.call(ctx, "dsn_estimateGas", params, &result); err != nil {
		return 0, err
	}

	return result.Gas, nil
}

// SendTransaction submits a signed transaction to the network.
func (c *Client) SendTransaction(ctx context.Context, tx *Transaction) (string, error) {
	params := SendTransactionParams{Transaction: *tx}

	var result SendTxResult
	if err := c.call(ctx, "dsn_sendTransaction", params, &result); err != nil {
		return "", err
	}

	return result.TxHash, nil
}

// SendRawTransaction submits a signed transaction (raw hex) to the network.
func (c *Client) SendRawTransaction(ctx context.Context, txHex string) (string, error) {
	// Create a minimal transaction struct with just the data
	tx := Transaction{Data: txHex}
	params := SendTransactionParams{Transaction: tx}

	var result SendTxResult
	if err := c.call(ctx, "dsn_sendTransaction", params, &result); err != nil {
		return "", err
	}

	return result.TxHash, nil
}

// GetEvents retrieves events matching a filter.
func (c *Client) GetEvents(ctx context.Context, filter EventFilter) ([]Event, error) {
	params := GetEventsParams{EventFilter: filter}

	var result []Event
	if err := c.call(ctx, "dsn_getEvents", params, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetValidators retrieves validator list for an epoch.
func (c *Client) GetValidators(ctx context.Context, epoch *uint64) ([]Validator, error) {
	params := GetValidatorsParams{Epoch: epoch}

	var result []Validator
	if err := c.call(ctx, "dsn_getValidators", params, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetCurrentValidators retrieves the current validator list.
func (c *Client) GetCurrentValidators(ctx context.Context) ([]Validator, error) {
	return c.GetValidators(ctx, nil)
}

// GetSupply retrieves token supply metrics.
func (c *Client) GetSupply(ctx context.Context) (*Supply, error) {
	var result Supply
	if err := c.call(ctx, "dsn_getSupply", nil, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetContractCode retrieves contract code by address.
func (c *Client) GetContractCode(ctx context.Context, address string) (string, error) {
	contract, err := c.GetContract(ctx, address)
	if err != nil {
		return "", err
	}

	return contract.CodeHash, nil
}