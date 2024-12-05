## Requirements
- `minikube`
- `kubectl`
- `docker`

## Problem

In Golang, create a system of gossiping nodes that, every 30 seconds, pulls and signs (using ECDSA) the price of ETH (from any freely available API e.g. CoinGecko), and emits it to a `libp2p` gossip network. `libp2p` nodes should successively sign the message and re-emit it to the network. When a message has received at least 3 signatures, and if it has been at least 30 seconds since the latest message from any node has been written, a node should write this message including its signatures to a shared Postgres instance. The schema you use is completely up to you but should be reasonable. Any node with the qualifying information can post to the database. The order of these signatures does not matter.

## Background

The project initializes a Postgres pod that creates the `eth_prices` table. Once the database instance is healthy, the network will start up. Each node will begin to listen on port 5000 and subscribe to the `eth_prices` gossip topic. It will discover the other peers in the (local) network using mDNS discovery and generate its own secret keypair. Finally, it connects to the Postgres instance and begins gossiping ETH prices to the network.

Every 30 seconds, each node will pull the current ETH/USD price from the public Coinbase API. Using the timestamp and price, the node hashes these values together and the resulting hash is signed by the node's private key. It is also appended to a list of other signatures in the message and broadcasts the message to the network. Once it's broadcasted, the other nodes will see and sign the message - if the message has the required minimum 3 signatures then the node will insert it into the shared Postgres instance. If another node tries to write the same message to the database, the insertion will be thrown out. The insertion will also fail if there is an entry inserted less than 30 seconds ago.

*This project was developed on an M2 Macbook Air.*

## Usage

This project was tested on a local Kubernetes cluster using `minikube`. Before running, decide the desired amount of nodes in the network by modifying the `replicas` field in `k8s/nodes-manifest.yaml`. By default, it is set to 3.

```
$ minikube start
$ eval $(minikube docker-env)
$ make
$ kubectl apply -f k8s/
```

## Logging

```
# View node logs
$ kubectl logs -f deployments/libp2p-nodes --all-containers

# View database contents
$ kubectl exec -it deployments/postgres-deployment -- psql -U postgres
postgres> SELECT * FROM eth_prices ORDER BY timestamp DESC;
```

## Potential Improvements 

Some improvements that could be made to the project with more time, for efficiency or security reasons.

- Send out a signal to the rest of the network that a message has been inserted to the database, ending the message propagation sooner before nodes execute commands on the database unnecessarily  
- Messages should require a signature from a validator node that also verifies that the signatures on a message belong to nodes on the network 
- Messages sent between nodes should be end-to-end encrypted
- Make the minimum signatures required to insert a message proportional to the number of nodes on the network
- Implement passing secrets into the deployments to secure the database contents
- Nodes should propogate to some of the other nodes instead of all of them to reduce traffic
- Implement fault tolerance for messages that may have a wildly different price from what the other nodes are reporting (may potentially be a bad actor)
