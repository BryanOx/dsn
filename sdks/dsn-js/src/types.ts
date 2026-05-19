// DSN TypeScript SDK — Type Definitions

export interface Block {
  number: number;
  hash: string;
  parentHash: string;
  timestamp: number;
  transactions: Transaction[];
  events: Event[];
}

export interface Transaction {
  hash: string;
  sender: string;
  recipient?: string;
  nonce: number;
  value: string;
  maxFee: number;
  gasLimit: number;
  data: string;
  signature?: string;
}

export interface TransactionReceipt {
  status: number;
  gasUsed: number;
  blockNumber: number;
  blockHash: string;
}

export interface Account {
  address: string;
  balance: string;
  nonce: number;
  codeHash: string;
  storageRoot: string;
}

export interface Contract {
  address: string;
  codeHash: string;
  metadata?: ContractMetadata;
}

export interface ContractMetadata {
  name: string;
  version: string;
  entrypoint: string;
}

export interface Event {
  contract: string;
  topics: string[];
  data: string;
  blockNumber: number;
  txHash?: string;
}

export interface Validator {
  address: string;
  power: number;
  commission: number;
}

export interface Supply {
  total: string;
  circulating: string;
  staked: string;
}

export interface EventFilter {
  contract?: string;
  topics?: string[];
  fromBlock?: number;
  toBlock?: number;
}

export interface CallResult {
  data: string;
  gasUsed: number;
}

export interface EstimateResult {
  gas: number;
}

export interface Pagination {
  offset: number;
  limit: number;
}

// JSON-RPC types
export interface RPCRequest {
  jsonrpc: string;
  method: string;
  params?: unknown[];
  id: number;
}

export interface RPCResponse<T = unknown> {
  jsonrpc: string;
  result?: T;
  error?: RPCError;
  id: number;
}

export interface RPCError {
  code: number;
  message: string;
  data?: unknown;
}
