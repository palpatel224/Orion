package node

type Node struct {
ID      string
Address string
Role    string
}

func NewNode(id, address, role string) *Node {
return &Node{
ID:      id,
Address: address,
Role:    role,
}
}
