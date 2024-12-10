package main

import (
	"math/rand"
	"time"
)

const UP_TIME time.Duration = 100 * time.Millisecond

const (
	CANDIDATE string = "CANDIDATE"
	FOLLOWER  string = "FOLLOWER"
	LEADER    string = "LEADER"
)

type Vote struct {
	Term    int
	Address string
}

type VoteReply struct {
	VotingFor string
}

type Heart struct {
	Leader string
	Term   int
}

type HeartReply struct {
	Ok bool
}

type Log struct {
	Command string
	Index   int
	Term    int
}

type LogReply struct {
	Ok bool
}

func RandomTimeout() time.Duration {
	return time.Duration((rand.Intn(300-150) + 150)) * time.Millisecond
}
