// https://en.wikipedia.org/wiki/Raft_(algorithm)#Leader_election
package main

import (
	"fmt"
	"log"
	"math/rand/v2"
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

	mu sync.Mutex
	c  chan bool
}

func (s *Server) Heartbeat(args *LeaderArgs, reply *LeaderReply) error {
	// TODO: I THINK THE ERROR HAPPENS BECAUSE THE CHECK IF THE LEADER IS VALID JUST OCCURS IN THE HEARTBET,
	// MAYBE, IF WE HAVE A MORE PRECISE WAY TO KNOW THIS WE CAN JUST GO TO A ELECTION RIGHT WAY
	s.mu.Lock()
	defer s.mu.Unlock()

	// leader got out
	if time.Since(s.LastHear) > 300*time.Millisecond || args.Term < s.TermNumber {
		s.NeedsLeader = true
		s.VotingFor = -1

		time.Sleep(RandomTimeout())
		go s.ElectionTimeout()
		return fmt.Errorf("leader is unreachable")
	}

	s.CurrentState = FOLLOWER
	s.NeedsLeader = false
	s.LastHear = time.Now()
	s.LeaderIndex = args.LeaderId
	log.Printf("[%d/%s] Heartbeat received from: %d", s.Id, s.CurrentState, args.LeaderId)

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
		s.TermNumber = args.CurrentTerm
		s.NeedsLeader = false

		s.LastHear = time.Now()
		return nil
	}

	reply.VotingFor = -1
	return nil
}

func (s *Server) ElectionTimeout() {
	s.mu.Lock()
	if !s.NeedsLeader {
		s.mu.Unlock()
		return
	}
	log.Printf("[%d] Participa da eleição", s.Id)
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
	log.Printf("[%d/%s] Sending Hearthbeat", s.Id, s.CurrentState)
	my_id := s.Id
	term := s.TermNumber
	s.mu.Unlock()

	errorChan := make(chan error, len(s.Servers))

	for _, v := range s.Servers {
		go func(c *rpc.Client) {
			args := LeaderArgs{LeaderId: my_id, Term: term}
			reply := LeaderReply{}
			err := c.Call("Server.Heartbeat", &args, &reply)
			errorChan <- err
		}(v)
	}

	for i := 0; i < len(s.Servers); i++ {
		if err := <-errorChan; err != nil {
			s.mu.Lock()
			s.VotingFor = -1
			s.NeedsLeader = true
			s.mu.Unlock()
			log.Printf("[%d] err: %s", s.Id, err)
			time.Sleep(RandomTimeout())
			go s.ElectionTimeout()
			return
		}
	}

	if rand.IntN(10) == 5 {
		log.Println("Atraso de 1s")
		time.Sleep(time.Second)
	}

	time.Sleep(100 * time.Millisecond)
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
	s.Servers = make(map[int]*rpc.Client)
	s.c = make(chan bool)
	s.VotingFor = -1
	s.CurrentState = FOLLOWER
	s.NeedsLeader = true
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
