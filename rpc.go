package main

import (
	"os"
	"strconv"
	"time"
)

type State string

const (
	FOLLOWER  = "FOLLOWER"
	LEADER    = "LEADER"
	CANDIDATE = "CANDIDATE"
)

const UP_TIME = 1 * time.Millisecond

func masterSock(id int) string {
	s := "/var/tmp/raft-"
	s += strconv.Itoa(os.Getuid())
	s += "-" + strconv.Itoa(id)
	return s
}
