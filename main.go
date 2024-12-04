package main

import (
	"fmt"
)

const port_start = 2000
const n_server = 5

func main() {

	fmt.Println("================ RAFT ================")

	for i := 0; i < n_server; i++ {
		s := &Server{}
		go s.Start(port_start+i, port_start, n_server)
	}

	select {}
}
