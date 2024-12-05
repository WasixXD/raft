package main

import (
	"math/rand"
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

type Vote struct {
	VotingFor    int
	CurrentState State
	CurrentTerm  int
	ServerId     int
}

type VoteReply struct {
	From      int
	VotingFor int
}

type LeaderArgs struct {
	LeaderId int
	Term     int
}

type LeaderReply struct {
}

func RandomTimeout() time.Duration {
	return time.Duration((rand.Intn(300-150) + 150)) * time.Millisecond

}

func masterSock(id int) string {
	s := "/var/tmp/raft-"
	s += strconv.Itoa(os.Getuid())
	s += "-" + strconv.Itoa(id)
	return s
}
