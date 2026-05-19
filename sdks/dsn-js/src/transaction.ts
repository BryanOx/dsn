import { DSNClient } from './client';
import type { Transaction } from './types';

export class TransactionBuilder {
  private sender: string = '';
  private recipient: string | undefined;
  private value: string = '0';
  private maxFee: number = 1_000_000;
  private gasLimit: number = 21_000;
  private data: string = '0x';
  private nonce?: number;

  constructor() {}

  /** Set the sender address. */
  from(address: string): this {
    this.sender = address;
    return this;
  }

  /** Set the recipient address (omitted for contract deployment). */
  to(address?: string): this {
    this.recipient = address;
    return this;
  }

  /** Amount in wei (string-encoded big integer). */
  amount(val: string): this {
    this.value = val;
    return this;
  }

  /** Max fee per gas. */
  fee(f: number): this {
    this.maxFee = f;
    return this;
  }

  /** Gas limit. Default 21 000 for simple transfers. */
  gas(g: number): this {
    this.gasLimit = g;
    return this;
  }

  /** Hex-encoded call data. */
  payload(hex: string): this {
    this.data = hex;
    return this;
  }

  /** Explicit nonce (fetched automatically if omitted). */
  setNonce(n: number): this {
    this.nonce = n;
    return this;
  }

  /** Build the transaction object. Nonce is resolved via the client if not set. */
  async build(client: DSNClient): Promise<Transaction> {
    const nonce =
      this.nonce ?? (await client.getNonce(this.sender));

    if (!this.sender) throw new Error('dsn: sender address required');

    return {
      hash: '', // filled by the node on submission
      sender: this.sender,
      recipient: this.recipient,
      nonce,
      value: this.value,
      maxFee: this.maxFee,
      gasLimit: this.gasLimit,
      data: this.data,
    };
  }

  /** Build and send in one step. Returns the transaction hash. */
  async send(client: DSNClient): Promise<string> {
    const tx = await this.build(client);
    return client.sendTransaction(tx);
  }
}
