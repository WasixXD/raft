package main

import (
	"log"
	"math/rand"
	"net/rpc"
	"os"
	"strconv"
	"time"
)

type State string

const (
	FOLLOWER  State = "FOLLOWER"
	LEADER          = "LEADER"
	CANDIDATE       = "CANDIDATE"
)

const UP_TIME = 1 * time.Millisecond

func RandomTimeout() time.Duration {
	return time.Duration((rand.Intn(300-150) + 150)) * time.Millisecond

}

func masterSock(id int) string {
	s := "/var/tmp/raft-"
	s += strconv.Itoa(os.Getuid())
	s += "-" + strconv.Itoa(id)
	return s
}

func Register() {
	err := rpc.Register(&Server{})

	if err != nil {
		log.Fatalln("Error on register: ", err)
		return
	}
	rpc.HandleHTTP()
}
