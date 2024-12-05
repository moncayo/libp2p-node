package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"math/big"

	"github.com/go-resty/resty/v2"
)

type PriceMessage struct {
	Price      float64     `json:"price"`
	Signatures []Signature `json:"signatures"`
	Timestamp  time.Time   `json:"timestamp"`
}

type Signature struct {
	NodeID string   `json:"node_id"`
	R      *big.Int `json:"r"`
	S      *big.Int `json:"s"`
}

type Rates struct {
	Currency string            `json:"currency"`
	Rates    map[string]string `json:"rates"`
}

type PriceData struct {
	Data Rates `json:"data"`
}

// Fetches the current ETH-USD price and passes it off to the gossiper.
func (n *Node) fetchAndGossipPrice(ctx context.Context) {
	client := resty.New()
	resp, err := client.R().Get(ApiURL)
	if err != nil {
		log.Printf("Failed to fetch price: %v", err)
		return
	}

	var priceData PriceData
	if err := json.Unmarshal(resp.Body(), &priceData); err != nil {
		log.Printf("fetchAndGossipPrice: Failed to unmarshal price data: %s", resp.String())
		return
	}

	price, err := strconv.ParseFloat(priceData.Data.Rates["USD"], 64)
	if err != nil {
		log.Printf("fetchAndGossipPrice: Failed to parse the float: %v", err)
		return
	}

	n.signAndGossip(ctx, PriceMessage{
		Price:     price,
		Timestamp: time.Now(),
	})
}

// Signs the price message and broadcasts it to the network.
func (n *Node) signAndGossip(ctx context.Context, msg PriceMessage) {
	// Signed hash of <price>-<timestamp>
	msgHash := sha256.Sum256([]byte(fmt.Sprintf("%f-%s", msg.Price, msg.Timestamp)))
	r, s, err := ecdsa.Sign(rand.Reader, n.privKey, msgHash[:])
	if err != nil {
		log.Printf("signAndGossip: Failed to sign message: %v", err)
		return
	}

	msg.Signatures = append(msg.Signatures, Signature{
		NodeID: n.host.String(),
		R:      r,
		S:      s,
	})

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("signAndGossip: Failed to marshal message: %v", err)
		return
	}

	if err := n.topic.Publish(ctx, msgBytes); err != nil {
		log.Printf("signAndGossip: Failed to publish message: %v", err)
	}
}

// Handles a new message and decides to write it to the database or
// re-propogate it to the network.
func (n *Node) handleMessage(ctx context.Context, msgBytes []byte) {
	var msg PriceMessage
	if err := json.Unmarshal(msgBytes, &msg); err != nil {
		log.Printf("handleMessage: Failed to unmarshal message: %v", err)
		return
	}

	if ts := n.LastEntry(ctx); msg.Timestamp.Sub(ts) <= PriceFetchInterval {
		// If the newest entry has been added and other valid messages are still
		// propagating, we can stop it from doing so unnecessarily
		log.Println("handleMessage: Found a recently posted entry, waiting for next fetch")
	} else if len(msg.Signatures) >= MinSignatures {
		if err := n.WriteToDatabase(ctx, msg); err != nil {
			log.Printf("handleMessage: Failed to write to database: %v", err)
		}
		log.Printf(
			"Successfully inserted timestamped price to database:\n"+
				" price: %f\n"+
				" timestamp: %v",
			msg.Price,
			msg.Timestamp,
		)
	} else {
		log.Println("handleMessage: not enough signatures, gossiping to network")
		n.signAndGossip(ctx, msg)
	}
}

// Retrieves the most recent timestamp from the database.
func (n *Node) LastEntry(ctx context.Context) time.Time {
	var ts time.Time
	query := "SELECT timestamp FROM eth_prices ORDER BY timestamp DESC LIMIT 1"
	err := n.database.QueryRow(ctx, query).Scan(&ts)

	if err != nil {
		log.Fatalf("LastEntry: could not get the latest timestamp from the database.")
	}

	return ts
}

// Inserts the qualifying message to the database.
// If the timestamp already exists in the database or the latest entry
// is younger than 30 seconds, the insertion will fail gracefully.
func (n *Node) WriteToDatabase(ctx context.Context, msg PriceMessage) error {
	query := `INSERT INTO eth_prices (price, timestamp, signatures)
              SELECT $1, $2, $3
              WHERE (SELECT MAX(timestamp) FROM eth_prices) < NOW() - INTERVAL '30 seconds'
                 OR (SELECT COUNT(*) FROM eth_prices) = 0
              ON CONFLICT (timestamp) DO NOTHING`
	_, err := n.database.Exec(ctx, query, msg.Price, msg.Timestamp, msg.Signatures)
	return err
}
