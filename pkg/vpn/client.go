package vpn

import (
	"net"
	"sync"

	"github.com/songgao/water"
)

type Client struct {
	newGateway string
	serverAddr string
	port       string

	wg              sync.WaitGroup
	serverIP        net.IP
	localAddr       net.IP
	additionalAddrs []net.IP
	localNetMask    *net.IPNet
	isShuttingDown  bool

	//channels between various components
	packetsIn     chan *RawIPPacket
	packetsDevOut chan *RawIPPacket

	tunInterface *water.Interface
	tcpConn      net.Conn

	// if false, packets are dropped
	connectionOk  bool
	connResetLock sync.Mutex
}
