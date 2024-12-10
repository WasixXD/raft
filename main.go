package main

import (
	"fmt"
	"net/rpc"
)

type Args struct {
}

const NUM_SERVERS int = 3
const PORT_START int = 2000

func main() {
	for i := 0; i < NUM_SERVERS; i++ {
		addr := fmt.Sprintf(":%d", PORT_START+i)
		node := Node{Address: addr, peers: make(map[string]*rpc.Client)}
		go node.Start(PORT_START, NUM_SERVERS)
	}

	select {}
}
