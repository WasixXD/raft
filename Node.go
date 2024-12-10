package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Node struct {
	Address string
	log     map[int]string
	peers   map[string]*rpc.Client
	kv      int
	state   string
	leader  string

	electionTimeout time.Duration
	TermNumber      int
	NeedsLeader     bool
	Voted           bool

	mu sync.Mutex
}

func (n *Node) processLog() {
	n.mu.Lock()
	defer n.mu.Unlock()

	lastLogKey := len(n.log) - 1
	lastLog := n.log[lastLogKey]

	command := strings.Split(lastLog, ":")

	switch command[0] {
	case "PUT":
		marks, err := strconv.Atoi(command[1])
		if err != nil {
			delete(n.log, lastLogKey)
			return
		}
		n.kv = marks
	}

}

func (n *Node) ReceiveAppend(args *Log, reply *LogReply) error {
	n.mu.Lock()

	if n.TermNumber == args.Term {
		n.log[args.Index] = args.Command
		reply.Ok = true
		n.mu.Unlock()
		n.processLog()

		return nil
	}

	n.mu.Unlock()
	reply.Ok = false

	return nil
}

func (n *Node) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		n.mu.Lock()
		addr := n.leader
		state := n.state
		n.mu.Unlock()

		if state != LEADER {

			leaderPath := fmt.Sprintf("http://localhost%s%s", addr, req.URL)

			proxyReq, err := http.NewRequest(req.Method, leaderPath, req.Body)
			if err != nil {
				log.Println("Could not reach leader: ", err)
			}

			proxyReq.Header = req.Header.Clone()

			res, err := http.DefaultClient.Do(proxyReq)

			if err != nil {
				log.Println("Err on request: ", err)
			}

			defer res.Body.Close()
			for key, values := range res.Header {
				for _, value := range values {
					w.Header().Add(key, value)
				}
			}
			w.WriteHeader(res.StatusCode)
			io.Copy(w, res.Body)

			return
		}
		next(w, req)
	}
}

func (n *Node) PutValue(w http.ResponseWriter, req *http.Request) {
	n.mu.Lock()
	a := n.Address
	n.mu.Unlock()

	fmt.Printf("Leader recebeu\n")
	body, _ := io.ReadAll(req.Body)
	commit := n.sendAppend(string(body))

	if commit {
		n.processLog()
		log.Printf("[%s] All commited", a)
		fmt.Fprintf(w, "OK\n")
		return
	}

	log.Printf("[%s] Something went wrong on commiting", a)
	fmt.Fprintf(w, "Something went Wrong\n")
}

func (n *Node) sendAppend(body string) bool {
	n.mu.Lock()
	i := len(n.log)
	term := n.TermNumber
	oks := 0
	canCommit := false
	n.mu.Unlock()

	wait := sync.WaitGroup{}

	for _, v := range n.peers {
		wait.Add(1)
		go func(c *rpc.Client) {
			defer wait.Done()
			n.mu.Lock()
			defer n.mu.Unlock()

			args := Log{Index: i, Command: body, Term: term}
			reply := LogReply{}

			c.Call("Node.ReceiveAppend", &args, &reply)

			if reply.Ok {
				oks++
			}

			if oks >= (len(n.peers)+1)/2 {
				n.log[i] = body
				canCommit = true
			}

		}(v)
	}
	wait.Wait()

	return canCommit
}

func (n *Node) ReceiveHeartbeat(args *Heart, reply *HeartReply) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if args.Term == n.TermNumber {
		log.Printf("[%s] received heartbeat from %s | MY KV: %d |", n.Address, args.Leader, n.kv)
		n.state = FOLLOWER
		n.leader = args.Leader
		reply.Ok = true
		return nil
	}

	n.NeedsLeader = true
	n.Voted = false
	reply.Ok = false

	return nil
}

func (n *Node) RequestVote(args *Vote, reply *VoteReply) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if args.Term > n.TermNumber && !n.Voted {
		log.Printf("[%s] voting for %s my term %d his term %d", n.Address, args.Address, n.TermNumber, args.Term)
		// set the reply
		reply.VotingFor = args.Address

		// update our information
		n.NeedsLeader = false
		n.Voted = true
		n.TermNumber = args.Term
	}

	return nil
}

func (n *Node) sendHeartbeat() {
	n.mu.Lock()
	duration := n.electionTimeout
	correct := true
	n.mu.Unlock()

	wait := sync.WaitGroup{}
	for {

		if !correct {
			break
		}

		for _, v := range n.peers {
			wait.Add(1)
			go func(c *rpc.Client) {
				n.mu.Lock()
				defer n.mu.Unlock()
				defer wait.Done()
				args := Heart{Term: n.TermNumber, Leader: n.Address}
				reply := HeartReply{}
				c.Call("Node.ReceiveHeartbeat", &args, &reply)

				if !reply.Ok {
					correct = false
					return
				}
			}(v)
		}
		wait.Wait()
		time.Sleep(duration)
	}
}

func (n *Node) election() {
	n.mu.Lock()

	n.TermNumber++
	n.state = CANDIDATE

	addr := n.Address
	term := n.TermNumber
	n.mu.Unlock()

	votes := 0
	wait := sync.WaitGroup{}

	for _, v := range n.peers {
		wait.Add(1)
		go func(c *rpc.Client) {
			n.mu.Lock()
			defer n.mu.Unlock()
			defer wait.Done()

			args := Vote{Term: term, Address: addr}
			reply := VoteReply{}
			c.Call("Node.RequestVote", &args, &reply)

			if reply.VotingFor == addr {
				votes++
			}

			if votes >= (len(n.peers)+1)/2 {
				n.state = LEADER
				return
			}
		}(v)
	}
	wait.Wait()
	if n.state == LEADER {
		log.Printf("[%s] gained the election", n.Address)
		go n.sendHeartbeat()
	}
}

func (n *Node) wait() {

	for {
		time.Sleep(n.electionTimeout)

		n.mu.Lock()
		needs := n.NeedsLeader
		n.mu.Unlock()

		if needs {
			n.election()
			break
		}
	}
}

func (n *Node) Start(portStart, numServers int) {
	n.mu = sync.Mutex{}
	n.electionTimeout = RandomTimeout()
	n.NeedsLeader = true
	n.log = make(map[int]string)

	// start both RPC and HTTP servers
	// new Http Multiplexer -> Telling go to let us do the way we want
	mux := http.NewServeMux()

	mux.HandleFunc("/put", n.Middleware(n.PutValue))

	rpcServer := rpc.NewServer()
	err := rpcServer.Register(n)

	if err != nil {
		log.Fatalln("Error on register: ", err)
	}

	l, e := net.Listen("tcp", n.Address)

	if e != nil {
		log.Fatalln("Error on RPC server: ", e)
	}
	mux.Handle("/_goRPC_", rpcServer)

	go http.Serve(l, mux)
	time.Sleep(UP_TIME)

	for i := 0; i < numServers; i++ {
		port := portStart + i
		portS := fmt.Sprintf(":%d", port)

		if portS == n.Address {
			continue
		}

		client, err := rpc.DialHTTP("tcp", portS)

		if err != nil {
			log.Fatalln("Error on connecting: ", err)
		}

		n.peers[portS] = client
	}
	log.Printf("[%s] up and running", n.Address)
	n.wait()
}
