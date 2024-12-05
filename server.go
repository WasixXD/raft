// https://en.wikipedia.org/wiki/Raft_(algorithm)#Leader_election
package main

import (
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"sync"
	"time"
)

type Server struct {
	Id         int
	TermNumber int
	VotingFor  int

	CurrentState State
	Servers      map[int]*rpc.Client
	LastHear     time.Time
	NeedsLeader  bool
	LeaderIndex  int

	mu   sync.Mutex
	cond sync.Cond
}

func (s *Server) Heartbeat(args *LeaderArgs, reply *LeaderReply) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.NeedsLeader = false
	s.LastHear = time.Now()
	s.LeaderIndex = args.LeaderId
	log.Printf("[%d/%s] Heartbeat received from: %d", s.Id, s.CurrentState, args.LeaderId)
	s.CurrentState = FOLLOWER

	return nil
}

func (s *Server) RequestVote(args *Vote, reply *VoteReply) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	reply.From = s.Id

	if args.CurrentTerm > s.TermNumber && s.VotingFor == -1 {
		// Set our vote so we only vote once
		s.VotingFor = args.ServerId

		// respond saying for who we vote for
		reply.VotingFor = args.ServerId

		// now we just follow the leader
		s.CurrentState = FOLLOWER
		return nil
	}

	reply.VotingFor = -1
	return nil
}

func (s *Server) ElectionTimeout() {
	s.mu.Lock()

	s.CurrentState = CANDIDATE
	s.TermNumber++
	s.VotingFor = s.Id

	voting_for := s.VotingFor
	current_term := s.TermNumber
	server_id := s.Id
	current_state := s.CurrentState

	votes_to_me := 0

	s.mu.Unlock()

	// send vote request
	for _, v := range s.Servers {
		go func(c *rpc.Client) {
			// count the votes
			vote := s.CallRequestVote(c, voting_for, current_term, server_id, current_state)
			s.mu.Lock()
			defer s.mu.Unlock()

			if vote == -1 || vote != s.Id || s.CurrentState != CANDIDATE {
				return
			}

			votes_to_me++

			if votes_to_me <= (len(s.Servers)+1)/2 {
				return
			}

			s.CurrentState = LEADER
			log.Printf("[%d] I gained the majority of votes VOTES=%d", s.Id, votes_to_me)
			go s.SendHeartbeat()
		}(v)
	}
}

func (s *Server) SendHeartbeat() {
	s.mu.Lock()
	log.Printf("[%d:%s] Sending Hearthbeat", s.Id, s.CurrentState)
	my_id := s.Id
	s.mu.Unlock()
	for _, v := range s.Servers {
		go func(c *rpc.Client) {
			args := LeaderArgs{LeaderId: my_id}
			reply := LeaderReply{}
			c.Call("Server.Heartbeat", &args, &reply)
		}(v)
	}
	time.Sleep(time.Second)
	go s.SendHeartbeat()
}

func (s *Server) CallRequestVote(client *rpc.Client, voting_for, current_term, server_id int, current_state State) int {
	args := Vote{VotingFor: voting_for, CurrentState: current_state, CurrentTerm: current_term, ServerId: server_id}
	reply := VoteReply{}
	client.Call("Server.RequestVote", &args, &reply)
	return reply.VotingFor
}

func (s *Server) Start(id, port_start, n_server int) {

	s.mu = sync.Mutex{}

	s.mu.Lock()
	s.Id = id
	s.cond = *sync.NewCond(&s.mu)
	s.Servers = make(map[int]*rpc.Client)
	s.VotingFor = -1
	s.CurrentState = FOLLOWER
	s.mu.Unlock()

	sockname := masterSock(id)
	os.Remove(sockname)

	listener, err := net.Listen("unix", sockname)

	if err != nil {
		log.Fatalln("Error on listening: ", err)
	}

	rpcServer := rpc.NewServer()
	err = rpcServer.Register(s)

	if err != nil {
		log.Fatalln("Error on register: ", err)
		return
	}

	mux := http.NewServeMux()
	mux.Handle("/_goRPC_", rpcServer)
	mux.Handle("/debug/rpc", http.DefaultServeMux)
	go http.Serve(listener, mux)
	time.Sleep(UP_TIME)

	for i := 0; i < n_server; i++ {
		port := port_start + i
		if port == id {
			continue
		}
		client, err := rpc.DialHTTP("unix", masterSock(port))
		if err != nil {
			log.Println("Error, couldn't reach server: ", err)
		}
		s.Servers[port] = client
	}
	log.Printf("[%d] up and running\n", id)
	time.Sleep(RandomTimeout())

	go s.ElectionTimeout()
}
