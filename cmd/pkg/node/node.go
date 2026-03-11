package node

import (
    workerpkg "orchestrator/worker"
)

type Node struct {
   Name string
   ID string
   Address string
   Role string
   TaskCount int
   Stats *workerpkg.Stats
}
func NewNode(id, address, role string,stat *workerpkg.Stats) *Node {
	return &Node{
		ID:      id,
        Address: address,
        Role:    role,
        Stats:   stat,
    }
}
