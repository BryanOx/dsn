.PHONY: help devnet-up devnet-down devnet-reset devnet-logs devnet-init devnet-status

help:
	@echo "Localnet Makefile Commands:"
	@echo ""
	@echo "  make devnet-init      Initialize localnet (generate keys, genesis, configs)"
	@echo "  make devnet-up        Start all localnet nodes"
	@echo "  make devnet-down      Stop all localnet nodes"
	@echo "  make devnet-reset    Stop, remove volumes, and re-initialize"
	@echo "  make devnet-logs     Tail logs from all nodes"
	@echo "  make devnet-status   Show running container status"
	@echo ""

devnet-init:
	@echo "Initializing localnet..."
	@chmod +x init.sh
	@./init.sh 3

devnet-up: devnet-init
	@echo "Starting localnet nodes..."
	@cd tmp && docker compose -f ../docker-compose.yml up -d
	@echo "Localnet started!"
	@echo "  node0: P2P :26656, RPC :8545, Metrics :9464"
	@echo "  node1: P2P :26657, RPC :8546, Metrics :9465"
	@echo "  node2: P2P :26658, RPC :8547, Metrics :9466"

devnet-down:
	@echo "Stopping localnet nodes..."
	@cd tmp && docker compose -f ../docker-compose.yml down || true

devnet-reset:
	@echo "Resetting localnet..."
	@cd tmp && docker compose -f ../docker-compose.yml down -v || true
	@rm -rf tmp
	@echo "Localnet reset. Run 'make devnet-up' to restart."

devnet-logs:
	@cd tmp && docker compose -f ../docker-compose.yml logs -f

devnet-status:
	@cd tmp && docker compose -f ../docker-compose.yml ps