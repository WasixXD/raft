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

var once sync.Once

type Server struct {
	Id           int
	TermNumber   int
	CurrentState State
	Servers      []*rpc.Client
	LastHear     time.Time
}

type Args struct{}
type ArgsReply struct {
	Reply int
}

func (s *Server) GetId(args *Args, reply *ArgsReply) error {
	reply.Reply = 500
	return nil
}

func (s *Server) MainLoop() {
	s.CurrentState = FOLLOWER
	for {
		s.LastHear = time.Now()

		time.Sleep(RandomTimeout())

		result := time.Since(s.LastHear)
		log.Println(result)
	}

}

func (s *Server) Start(id, port_start, n_server int) {
	once.Do(Register)
	s.Id = id

	sockname := masterSock(id)
	os.Remove(sockname)

	listener, err := net.Listen("unix", sockname)

	if err != nil {
		log.Fatalln("Error on listening: ", err)
	}

	go http.Serve(listener, nil)
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
		s.Servers = append(s.Servers, client)
	}
	log.Printf("[%d] up and running\n", id)
	s.MainLoop()
}
