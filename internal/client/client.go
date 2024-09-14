package client

import (
	"net"
	"sync"
)

type Client struct {
	x     float64
	y     float64
	z     float64
	vel   float64
	conn  net.Conn
	mutex sync.RWMutex
}

func NewClient(c net.Conn) *Client {
	return &Client{
		x:     0,
		y:     0,
		z:     0,
		vel:   0,
		conn:  c,
		mutex: sync.RWMutex{},
	}
}
