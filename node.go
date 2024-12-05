package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

type Node struct {
	host     peer.ID
	pubsub   *pubsub.PubSub
	sub      *pubsub.Subscription
	topic    *pubsub.Topic
	privKey  *ecdsa.PrivateKey
	database *pgxpool.Pool
}

// Bootstraps node with necessary connections.
func BootstrapNode(ctx context.Context) *Node {
	log.Println("Initializing node...")

	// Create a new libp2p host
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/5000"))
	if err != nil {
		log.Fatalf("Failed to create libp2p host: %v", err)
	}

	// Join the pubsub topic
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		log.Fatalf("Failed to create pubsub: %v", err)
	}

	if err := setupDiscovery(h); err != nil {
		log.Fatalf("Failed to set up peer discovery: %v", err)
	}

	topic, err := ps.Join(GossipTopic)
	if err != nil {
		log.Fatalf("Failed to join topic: %v", err)
	}
	log.Printf("Joined topic: %v\n", topic)

	sub, err := topic.Subscribe()
	if err != nil {
		log.Fatalf("Failed to subscribe to topic: %v", err)
	}
	log.Printf("Subscribed to topic: %v\n", sub.Topic())

	// Generate a private key for signing
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate private key: %v", err)
	}
	log.Printf("Generated private key: %v\n", privKey)

	// Initialize PostgreSQL pool connection
	conn, err := pgxpool.Connect(ctx, DatabaseURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	log.Println("Connected to database!")

	return &Node{
		host:     h.ID(),
		pubsub:   ps,
		sub:      sub,
		topic:    topic,
		privKey:  privKey,
		database: conn,
	}
}

// Initialize message listener and gossiper.
func (n *Node) Start(ctx context.Context) {
	defer n.database.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	go n.Gossip(ctx)
	go n.Listen(ctx)

	wg.Wait()
}

func (n *Node) Gossip(ctx context.Context) {
	log.Println("Starting gossiper...")
	ticker := time.NewTicker(PriceFetchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.fetchAndGossipPrice(ctx)
		}
	}
}

func (n *Node) Listen(ctx context.Context) {
	log.Println("Starting listener...")

	for {
		msg, err := n.sub.Next(ctx)
		if err != nil {
			log.Fatalf("Something went wrong with the listener: %v", err)
		}

		// only consider messages delivered by other peers
		if msg.ReceivedFrom == n.host {
			continue
		}

		log.Printf("received new message from: %s\n", msg.ReceivedFrom.String())

		n.handleMessage(ctx, msg.Data)
	}
}

// setupDiscovery creates an mDNS discovery service and attaches it to the libp2p Host.
// This lets us automatically discover peers on the same LAN and connect to them.
func setupDiscovery(h host.Host) error {
	// setup mDNS discovery to find local peers
	s := mdns.NewMdnsService(h, GossipTopic, &discoveryNotifee{h: h})
	return s.Start()
}

// discoveryNotifee gets notified when we find a new peer via mDNS discovery.
type discoveryNotifee struct {
	h host.Host
}

// HandlePeerFound connects to peers discovered via mDNS. Once they're connected,
// the PubSub system will automatically start interacting with them if they also
// support PubSub.
func (n *discoveryNotifee) HandlePeerFound(p peer.AddrInfo) {
	log.Printf("Discovered new peer: %s\n", p.ID)
	err := n.h.Connect(context.Background(), p)
	if err != nil {
		log.Printf("Error connecting to peer %s: %s\n", p.ID, err)
	}
}
