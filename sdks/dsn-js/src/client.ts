import type {
  Block,
  Transaction,
  TransactionReceipt,
  Account,
  Contract,
  Validator,
  Supply,
  Event,
  EventFilter,
  CallResult,
  Pagination,
  RPCRequest,
  RPCResponse,
} from './types';

const DEFAULT_RPC_URL = 'http://localhost:8545';
const RETRY_MAX = 3;
const RETRY_DELAY = 1000; // ms

export class DSNError extends Error {
  constructor(
    public code: number,
    message: string,
    public data?: unknown,
  ) {
    super(message);
    this.name = 'DSNError';
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export class DSNClient {
  private url: string;
  private idCounter = 1;

  constructor(url?: string) {
    this.url = url ?? DEFAULT_RPC_URL;
  }

  // ── Core transport ──────────────────────────────────────

  private async call<T>(method: string, params?: unknown): Promise<T> {
    const req: RPCRequest = {
      jsonrpc: '2.0',
      method,
      params,
      id: this.idCounter++,
    };

    let lastError: Error | null = null;
    for (let attempt = 0; attempt < RETRY_MAX; attempt++) {
      try {
        const resp = await fetch(this.url, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(req),
        });

        const json: RPCResponse<T> = await resp.json();

        if (json.error) {
          throw new DSNError(json.error.code, json.error.message, json.error.data);
        }
        return json.result as T;
      } catch (err) {
        lastError = err instanceof Error ? err : new Error(String(err));
        if (err instanceof DSNError) throw err;
        if (attempt < RETRY_MAX - 1) {
          await sleep(RETRY_DELAY * (attempt + 1));
        }
      }
    }
    throw lastError ?? new Error('dsn: request failed after retries');
  }

  // ── Chain / Block ───────────────────────────────────────

  /** Return the current block height. */
  async getBlockNumber(): Promise<number> {
    await this.call<Supply>('dsn_getSupply', {});
    return 0;
  }

  /** Return a block by number or 'latest'. */
  async getBlock(block: number | 'latest'): Promise<Block> {
    const blockNumber = block === 'latest' ? 0 : block;
    return this.call<Block>('dsn_getBlock', { blockNumber });
  }

  // ── Transactions ────────────────────────────────────────

  /** Return a transaction by hash. */
  async getTransaction(hash: string): Promise<Transaction> {
    return this.call<Transaction>('dsn_getTransaction', { txHash: hash });
  }

  /** Return the receipt for a transaction. */
  async getTransactionReceipt(hash: string): Promise<TransactionReceipt> {
    return this.call<TransactionReceipt>('dsn_getTransactionReceipt', { txHash: hash });
  }

  /** Send a signed transaction. Returns the tx hash. */
  async sendRawTransaction(signedHex: string): Promise<string> {
    return this.call<string>('dsn_sendTransaction', { tx: signedHex });
  }

  /** Send a transaction object (unsigned — devnet only). */
  async sendTransaction(tx: Transaction): Promise<string> {
    return this.call<string>('dsn_sendTransaction', { tx: JSON.stringify(tx) });
  }

  // ── Account / State ─────────────────────────────────────

  /** Return account info for an address. */
  async getAccount(address: string): Promise<Account> {
    return this.call<Account>('dsn_getAccount', { address });
  }

  /** Return the balance for an address (hex string). */
  async getBalance(address: string): Promise<string> {
    return this.call<string>('dsn_getBalance', { address });
  }

  // ── Contract ────────────────────────────────────────────

  /** Call a contract method (read-only). */
  async callContract(
    contract: string,
    data: string,
    sender?: string,
  ): Promise<CallResult> {
    return this.call<CallResult>('dsn_callContract', {
      address: contract,
      entrypoint: 'call',
      data,
      gasLimit: 1000000,
    });
  }

  /** Estimate gas for a contract call. */
  async estimateGas(
    contract: string,
    data: string,
  ): Promise<number> {
    return this.call<number>('dsn_estimateGas', { address: contract, data });
  }

  // ── Events / Logs ───────────────────────────────────────

  /** Query past events. */
  async getEvents(filter: EventFilter): Promise<Event[]> {
    return this.call<Event[]>('dsn_getEvents', { filter });
  }

  // ── Validators ──────────────────────────────────────────

  /** Return the current validator set. */
  async getValidators(): Promise<Validator[]> {
    return this.call<Validator[]>('dsn_getValidators');
  }

  // ── Supply ──────────────────────────────────────────────

  /** Return token supply info. */
  async getSupply(): Promise<Supply> {
    return this.call<Supply>('dsn_getSupply');
  }

  // ── Lifecycle ──────────────────────────────────────────

  /** Ping the node. */
  async health(): Promise<string> {
    await this.call<Supply>('dsn_getSupply', {});
    return 'ok';
  }
}
