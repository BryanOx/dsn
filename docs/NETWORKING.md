# P2P Networking

## Overview

The DSN networking layer provides peer-to-peer communication for transaction propagation, block distribution, consensus voting, and state synchronization. It implements a partial mesh topology with configurable peer counts, gossip protocols for efficient message dissemination, and rate limiting to prevent abuse.

The networking system is composed of:

- **network/p2p.go** — Core P2P node implementation
- **network/gossip.go** — Gossip protocol for block/transaction propagation
- **network/message.go** — Message framing and type definitions
- **network/peer.go** — Peer management and metadata
- **network/peer_discovery.go** — Peer discovery via bootstrap nodes
- **network/discovery.go** — Discovery utilities
- **network/blocksync.go** — Block synchronization
- **network/fastsync.go** — State snapshot synchronization

---

## Key Concepts

- **P2P Node**: A network participant that connects to other nodes, sends and receives messages. Each node has a unique ID derived from its listen address.

- **Peer**: A connected node. Peers are managed via the PeerManager with state tracking (connected, disconnected, banned).

- **Gossip Protocol**: A dissemination mechanism where nodes forward messages to a subset of peers. This provides efficient propagation without flooding the network.

- **Message Framing**: All messages use a length-prefixed format: `[4-byte length][1-byte type][payload]`.

- **Token Bucket**: A rate limiting algorithm that allows burst traffic up to a limit while refilling at a specified rate.

- **SeenSet**: A data structure tracking recently seen messages to prevent duplicate processing.

---

## Architecture

### P2P Node (network/p2p.go)

The P2PNode handles TCP connections:

```go
type P2PNode struct {
    addr          string
    listener      net.Listener
    pm            *PeerManager
    connections   map[string]net.Conn
    txHandler     func(tx *types.Transaction)
    blockHandler  func(data []byte)
    stopCh        chan struct{}
    // Rate limiting
    connLimiter   *connectionLimiter
    rateLimiters  map[string]*TokenBucket
    // Engines
    fastSync      *FastSyncEngine
    blockSync     *BlockSyncEngine
    gossip        *GossipEngine
    discovery     *PeerDiscovery
    // Block processing
    blockCh       chan blockJob
}
```

**Connection handling:**
- Accepts incoming connections up to max peers (50)
- Per-IP rate limiting (5 events/sec, burst 5)
- Message reading with 30-second deadline
- Per-peer rate limiting (100 tokens, 10/sec refill)

### Peer Manager

Manages peer metadata and scoring:

```go
type PeerManager struct {
    peers map[PeerID]*Peer
    // ...
}
```

- Tracks peer state (connected, disconnected, banned)
- Records failure events for reputation
- Applies temporary bans for abuse (5-minute default)

### Message Types

All protocol messages use a type byte (network/message.go):

| Type | Value | Purpose |
|------|-------|---------|
| MsgTypeBlock | 0x01 | Block propagation |
| MsgTypeTransaction | 0x02 | Transaction propagation |
| MsgTypeSnapshotQuery | 0x10 | Query available snapshots |
| MsgTypeSnapshotInfo | 0x11 | Snapshot metadata response |
| MsgTypeSnapshotRequest | 0x12 | Request snapshot chunk |
| MsgTypeSnapshotChunk | 0x13 | Snapshot chunk data |
| MsgTypePeerExchange | 0x20 | Peer address exchange |
| MsgTypeBlockRangeRequest | 0x30 | Request block range |
| MsgTypeBlockRangeResponse | 0x31 | Block range response |
| MsgTypePing | 0x40 | Keepalive ping |
| MsgTypePong | 0x41 | Keepalive pong |

### Gossip Engine (network/gossip.go)

The GossipEngine implements fan-out propagation:

- **Block gossip**: Fan-out to `max(3, sqrt(N))` random peers
- **Transaction gossip**: Broadcast to all connected peers
- **Dedup**: SeenSet tracks recent messages to prevent reprocessing

**Rate limiting:**
- Per-peer token buckets (100 tokens, 10/sec)
- Cleanup of stale buckets every 5 minutes

---

## Topology

### Partial Mesh

DSN uses a partial mesh topology:
- Each node connects to multiple peers (configurable, max 50)
- Not all nodes connect to all other nodes
- Connections are asymmetric (Node A may connect to B without B connecting to A)

### Peer Discovery

Peer discovery is handled by PeerDiscovery (network/peer_discovery.go):
- **Bootstrap nodes**: Initial peers provided via configuration
- **Peer exchange**: Nodes share known peers via MsgTypePeerExchange
- **Connection attempts**: Exponential backoff with jitter on failures

---

## Gossip Protocol

### Transaction Propagation

```
Incoming TX → Compute hash → Check SeenSet → Mark Seen → Broadcast to peers
```

- Transaction gossiped to all connected peers
- Each peer checks rate limit before accepting
- SeenSet prevents duplicate processing

### Block Propagation

```
Incoming Block → Compute hash → Check SeenSet → Mark Seen → Fan-out to √N peers
```

- Fan-out count: `max(3, sqrt(connected_peers))`
- Selected peers via Fisher-Yates shuffle with crypto/rand
- Each peer checked against rate limiter

### Vote Propagation

Votes are propagated via the consensus gossip mechanism (consensus/gossip.go):
- Validators broadcast pre-votes and pre-commits
- Messages are deduplicated via SeenSet
- Rate limiting applied per peer

---

## Rate Limiting

### Connection Limiter

Prevents connection flooding:

```go
type connectionLimiter struct {
    maxPeers    int           // 50
    perIPRate   rate.Limit    // 5 events/sec
    burst       int           // 5
    tempBans    map[string]time.Time
}
```

- Checks temp ban list (60-second default)
- Enforces max concurrent peers
- Per-IP token bucket rate limiting

### Per-Peer Rate Limiter

Prevents message flooding from individual peers:

```go
type TokenBucket struct {
    maxTokens  float64  // 100
    refillRate float64  // 10 per second
}
```

- Applied to incoming messages
- Penalty applied on rate limit exceeded
- Repeated violations trigger ban (5 minutes)

### Token Bucket Implementation

```go
func (tb *TokenBucket) Allow() bool {
    tb.refill()
    if tb.tokens >= 1 {
        tb.tokens -= 1
        return true
    }
    return false
}
```

Refill adds tokens based on elapsed time:
```go
tb.tokens = math.Min(tb.maxTokens, tb.tokens + tb.refillRate*elapsed.Seconds())
```

---

## Synchronization

### Block Sync (network/blocksync.go)

Allows nodes to request missing blocks:
- MsgTypeBlockRangeRequest: Request a range of blocks
- MsgTypeBlockRangeResponse: Respond with blocks (max 100)

### Fast Sync (network/fastsync.go)

Allows nodes to download state snapshots:
- Query available snapshots (MsgTypeSnapshotQuery)
- Request snapshot chunks (MsgTypeSnapshotRequest)
- Receive chunk data (MsgTypeSnapshotChunk)
- Validator set provides integrity verification

---

## Reconnection

When peer connections fail:
1. Connection removed from map
2. PeerManager updated
3. Rate limiter cleaned up
4. Discovery attempts to reconnect via other peers

**Exponential backoff:**
- Initial delay: configurable
- Max delay: configurable
- Jitter: random to prevent thundering herd

---

## Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| Listen Port | Port for incoming connections | Configurable |
| Max Peers | Maximum concurrent connections | 50 |
| Connection Rate | Per-IP connection rate limit | 5/sec |
| Burst | Connection burst allowance | 5 |
| Message Rate | Per-peer message rate | 10/sec |
| Message Burst | Per-peer message burst | 100 |
| Max Payload | Maximum message size | 1 MiB |
| Read Timeout | Connection read deadline | 30 seconds |
| Ban Duration | Duration for temp bans | 5 minutes |

---

## Message Flow

### Sending a Transaction

1. Client submits transaction to local node
2. Node adds to mempool
3. GossipEngine broadcasts to peers
4. Peers validate and add to mempool
5. Process repeats until all nodes have transaction

### Receiving a Block

1. Connection receives framed message
2. Message type identified (0x01 = Block)
3. Rate limit checked (100 tokens, 10/sec)
4. Dedup check via SeenSet
5. Block marked as seen
6. Fan-out to √N random peers
7. Block handler invoked (consensus)

### Fast Sync

1. Node queries peers for snapshots (MsgTypeSnapshotQuery)
2. Receives snapshot metadata (MsgTypeSnapshotInfo)
3. Requests chunks (MsgTypeSnapshotRequest)
4. Receives chunks (MsgTypeSnapshotChunk)
5. Reconstructs state
6. Validates against checkpoint

---

## Troubleshooting

### Common Issues

| Issue | Cause | Resolution |
|-------|-------|------------|
| Connection refused | Peer not reachable | Check network, retry later |
| Max peers reached | Too many connections | Wait for peer disconnect |
| Rate limited | Excess traffic | Reduce message rate |
| Banned | Repeated violations | Wait for ban expiration |
| Stale peers | Network partition | Discovery will find new peers |

### Debugging

- Check peer count: `node.NumPeers()`
- Review peer states via PeerManager
- Monitor rate limiter metrics
- Check message type distribution
- Verify sync status

---

## Security

### Connection Security
- No TLS in current implementation (assumes trusted network)
- Peer validation via signature verification (consensus)
- Evidence-based slashing for malicious behavior

### Abuse Prevention
- Connection rate limiting per IP
- Message rate limiting per peer
- Duplicate message filtering
- Temporary bans for repeated violations

---

## Related Documentation

- [CONSENSUS.md](./CONSENSUS.md) — Consensus message propagation
- [PERSISTENCE.md](./PERSISTENCE.md) — State sync for fast sync
- [dsn_protocol_spec_v_1.md](./dsn_protocol_spec_v_1.md) — Protocol specification