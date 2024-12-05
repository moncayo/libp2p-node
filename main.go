package main

import (
	"context"
	"time"
)

// Globals
const (
	PriceFetchInterval = 30 * time.Second
	MinSignatures      = 3
	GossipTopic        = "eth-price"
	DatabaseURL        = "postgres://postgres:password@postgres-service.default.svc.cluster.local:5432/postgres"
	ApiURL             = "https://api.coinbase.com/v2/exchange-rates?currency=ETH"
)

func main() {
	ctx := context.Background()
	node := BootstrapNode(ctx)
	node.Start(ctx)
}
