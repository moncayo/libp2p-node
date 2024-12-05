build:
	docker build -t libp2p-node .
	docker build -t libp2p-postgres -f Dockerfile.db .
