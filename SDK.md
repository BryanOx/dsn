# DSN JavaScript SDK

Located at `sdks/dsn-js/`.

## Installation

```bash
cd sdks/dsn-js
npm install
npm run build
```

Published as `@dsn/sdk` (v0.1.0). Only dependency is `ws` (not yet used — SDK uses HTTP fetch only).

## Quick Start

```typescript
import { DSNClient, TransactionBuilder } from '@dsn/sdk';

const client = new DSNClient('http://localhost:8545');

// Check balance
const balance = await client.getBalance('16okjApjLUDg6PWZEqoBRXwdbnHoKwLgyr');
console.log('Balance:', balance);

// Send a transaction
const txHash = await new TransactionBuilder()
  .from('16okjApjLUDg6PWZEqoBRXwdbnHoKwLgyr')
  .to('DSN1recipient...')
  .amount('1000')
  .send(client);
```

## API Reference

### DSNClient

| Method | Params | Returns | RPC |
|--------|--------|---------|-----|
| `getAccount` | `address: string` | `Account` | `dsn_getAccount` |
| `getBalance` | `address: string` | `string` | `dsn_getBalance` |
| `getBlock` | `block: number \| 'latest'` | `Block` | `dsn_getBlock` |
| `getTransaction` | `hash: string` | `Transaction` | `dsn_getTransaction` |
| `sendTransaction` | `tx: Transaction` | `string` | `dsn_sendTransaction` |
| `sendRawTransaction` | `signedHex: string` | `string` | `dsn_sendTransaction` |
| `callContract` | `contract, data, sender?` | `CallResult` | `dsn_callContract` |
| `estimateGas` | `contract, data` | `number` | `dsn_estimateGas` |
| `getValidators` | — | `Validator[]` | `dsn_getValidators` |
| `getSupply` | — | `Supply` | `dsn_getSupply` |
| `getTransactionReceipt` | `hash: string` | `TransactionReceipt` | `dsn_getTransactionReceipt` |
| `getEvents` | `filter: EventFilter` | `Event[]` | `dsn_getEvents` |
| `getBlockNumber` | — | `number` (always `0`) | (stub) |
| `health` | — | `string` (`'ok'`) | (ping via `dsn_getSupply`) |

Transport: JSON-RPC 2.0 over HTTP POST with retry (3 attempts, exponential backoff).

### TransactionBuilder

Fluent builder for constructing transactions.

| Method | Params | Description |
|--------|--------|-------------|
| `from(address)` | `string` | Sender address (required) |
| `to(address?)` | `string` | Recipient (omit for contract deployment) |
| `amount(val)` | `string` | Amount (string-encoded big integer) |
| `fee(f)` | `number` | Max fee per gas |
| `gas(g)` | `number` | Gas limit (default 21,000) |
| `payload(hex)` | `string` | Hex-encoded call data |
| `setNonce(n)` | `number` | Explicit nonce (auto-fetched if omitted) |
| `build(client)` | `DSNClient` | `Promise<Transaction>` — resolves nonce if not set |
| `send(client)` | `DSNClient` | `Promise<string>` — build + send, returns tx hash |

Usage:

```typescript
const tx = await new TransactionBuilder()
  .from(sender)
  .to(recipient)
  .amount('5000')
  .fee(10)
  .gas(21000)
  .build(client);

const hash = await client.sendTransaction(tx);
```

## Known Limitations

- `getBlockNumber()` returns `0` — no server RPC for block height yet
- `getEvents()` requires an event indexer (not yet built on server side)
- `getTransactionReceipt()` returns pending for committed txs — no block indexer
- No built-in transaction signing (use `dsn tx sign` CLI or external tool)
- No WebSocket support (despite `ws` dependency; all calls use HTTP POST)

## See Also

Go SDK: `import "github.com/dsn/dsn/sdk"` (in-repo)
