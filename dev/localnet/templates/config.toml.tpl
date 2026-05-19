[p2p]
listen_addr = "0.0.0.0"
port = {{.P2PPort}}
max_peers = 50
bootstrap_peers = {{.BootstrapPeers}}
ping_interval = "30s"

[rpc]
enabled = true
listen_addr = "0.0.0.0"
port = {{.RPCPort}}
cors_origins = []

[metrics]
enabled = true
listen_addr = "0.0.0.0"
port = {{.MetricsPort}}

[storage]
data_dir = "/root/.dsn/data"
max_db_size = 10737418240

[snapshot]
enable = true
interval = 10
max_snapshots = 5

[validator]
key_file = "/root/.dsn/validator.key"
stake = {{.Stake}}
commission_rate = {{.CommissionRate}}

[logging]
level = "{{.LogLevel}}"
format = "{{.LogFormat}}"
output = "{{.LogOutput}}"

[chain]
chain_id = {{.ChainID}}
mempool_max_size = {{.MempoolMaxSize}}
mempool_ttl = "{{.MempoolTTL}}"
max_tx_per_block = {{.MaxTxPerBlock}}
proposer_timeout = "{{.ProposerTimeout}}"

[genesis]
file = "/root/genesis.json"