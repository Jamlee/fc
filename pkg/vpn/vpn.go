package vpn

import (
	"encoding/gob"
	"errors"
	"log"
	"net"
	"sync"
	"time"

	"github.com/songgao/water"
	"github.com/songgao/water/waterutil"
)

const (
	PacketInMaxBuff        = 150
	PacketOutMaxBuff       = 150
	devMtuSize             = 1500
	devPktBuffSize         = 4096
	devTxQueLen            = 300
	servMaxInboundPktQueue = 400
	servPerClientPktQueue  = 200

	// for connection
	PktUnknown PktType = iota
	PktIPPkt
	PktLocalAddr
)

type PktType byte
type OutBoundIPPacket InBoundIPPacket

type RawIPPacket struct {
	Raw      []byte
	Dest     net.IP
	Protocol waterutil.IPProtocol
}

type InBoundIPPacket struct {
	packet   *RawIPPacket
	clientID int
}

type Server struct {
	listener          net.Listener
	localAddr         net.IP
	localNetMask      *net.IPNet
	isShuttingDown    bool
	clientIDByAddress map[string]int
	clients           map[int]*ServerConn
	clientsLock       sync.Mutex

	inboundIPPkts chan *InBoundIPPacket

	// add encrypetd packet
	inboundEncryptedPkts  chan *RawIPPacket
	outboundEncryptedPkts chan *RawIPPacket

	tunInterface *water.Interface
	rm           RouterManager
	wg           sync.WaitGroup
	lastClientID int
}

type ServerConn struct {
	id               int
	conn             net.Conn
	outBoundIPPacket chan *OutBoundIPPacket
	canSendIP        bool
	remoteAddrs      []net.IP
	connectionOk     bool
	server           *Server
}

////////////////////////////////////////////////////////////////////////////////////////
//
//  Server
//
/////////////////////////////////////////////////////////////////////////////////////////

func NewServer(listenHost, listenPort, network, iName string) (*Server, error) {
	netIP, localNetMask, err := net.ParseCIDR(network)
	if err != nil {
		return nil, errors.New("invalid network address/mask - " + err.Error())
	}
	config := water.Config{
		DeviceType: water.TUN,
	}
	config.Name = iName
	tunInterface, err := water.New(config)
	if err != nil {
		log.Fatalf("can not created  vpn iface %s\n", iName)
	}
	log.Printf("created  vpn iface %s\n", tunInterface.Name())
	s := &Server{
		tunInterface:          tunInterface,
		localAddr:             netIP,
		localNetMask:          localNetMask,
		inboundIPPkts:         make(chan *InBoundIPPacket, servMaxInboundPktQueue),
		inboundEncryptedPkts:  make(chan *RawIPPacket, PacketInMaxBuff),
		outboundEncryptedPkts: make(chan *RawIPPacket, PacketOutMaxBuff),
		clientIDByAddress:     map[string]int{},
		clients:               map[int]*ServerConn{},
		lastClientID:          0,
	}
	return nil, s.Init(listenHost + ":" + listenPort)
}

func (s *Server) Init(addr string) (err error) {
	s.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	if err = SetDevIP(s.tunInterface.Name(), s.localAddr, s.localNetMask, false); err != nil {
		return err
	}
	return nil
}

func (s *Server) acceptRoutine() {
	var tcpListener *net.TCPListener
	tcpListener, _ = s.listener.(*net.TCPListener)
	s.wg.Add(1)
	defer s.wg.Done()

	for !s.isShuttingDown {
		if tcpListener != nil {
			tcpListener.SetDeadline(time.Now().Add(time.Millisecond * 300))
		}
		conn, err := s.listener.Accept()
		if err != nil {
			if !s.isShuttingDown {
				log.Printf("Listener err: %s\n", err.Error())
			}
			return
		}
		s.handleClient(conn)
	}
}

func (s *Server) handleClient(conn net.Conn) {
	c := ServerConn{
		conn:      conn,
		canSendIP: true,
	}
	s.enrollClientConn(&c)
	c.initClient(s)
}

func (s *Server) enrollClientConn(c *ServerConn) {
	s.clientsLock.Lock()
	defer s.clientsLock.Unlock()
	c.id = s.lastClientID
	s.lastClientID++
	s.clients[c.id] = c
}

func (s *Server) setAddrForClient(id int, addr net.IP) {
	s.clientsLock.Lock()
	defer s.clientsLock.Unlock()

	s.clientIDByAddress[addr.String()] = id
}

func (s *Server) removeClientConn(id int) {
	s.clientsLock.Lock()
	defer s.clientsLock.Unlock()

	//delete from the clientIDByAddress map if it exists
	var toDeleteAddrs []string
	for dest, itemID := range s.clientIDByAddress {
		if itemID == id {
			toDeleteAddrs = append(toDeleteAddrs, dest)
		}
	}
	for _, addr := range toDeleteAddrs {
		delete(s.clientIDByAddress, addr)
	}
	delete(s.clients, id)
}

////////////////////////////////////////////////////////////////////////////////////////
//
//  ServerConn
//
/////////////////////////////////////////////////////////////////////////////////////////

func (c *ServerConn) initClient(s *Server) {
	c.outBoundIPPacket = make(chan *OutBoundIPPacket, servPerClientPktQueue)
	c.connectionOk = true
	c.server = s
	log.Printf("New connection from %s (%d)\n", c.conn.RemoteAddr().String(), c.id)
	go c.readRoutine(&s.isShuttingDown, s.inboundIPPkts)
	go c.writeRoutine(&s.isShuttingDown)
}

func (c *ServerConn) writeRoutine(isShuttingDown *bool) {
	encoder := gob.NewEncoder(c.conn)
	for !*isShuttingDown && c.connectionOk {
		select {
		case pkt := <-c.outBoundIPPacket:
			encoder.Encode(PktIPPkt)
			err := encoder.Encode(pkt)
			if err != nil {
				log.Printf("Write error for %s: %s\n", c.conn.RemoteAddr().String(), err.Error())
				c.hadError(false)
				return
			}
		}
	}
}

func (c *ServerConn) readRoutine(isShuttingDown *bool, ipPacketSink chan *InBoundIPPacket) {
	decoder := gob.NewDecoder(c.conn)

	for !*isShuttingDown && c.connectionOk {
		var pktType PktType
		err := decoder.Decode(&pktType)
		if err != nil {
			if !*isShuttingDown {
				log.Printf("Client read error: %s\n", err.Error())
			}
			c.hadError(true)
			return
		}

		switch pktType {
		case PktLocalAddr:
			var localAddr net.IP
			err := decoder.Decode(&localAddr)
			if err != nil {
				log.Printf("Could not decode net.IP: %s", err.Error())
				c.hadError(false)
				return
			}
			c.remoteAddrs = append(c.remoteAddrs, localAddr)
			c.server.setAddrForClient(c.id, localAddr)

		case PktIPPkt:
			var ipPkt RawIPPacket
			err := decoder.Decode(&ipPkt)
			if err != nil {
				log.Printf("Could not decode IPPacket: %s", err.Error())
				c.hadError(false)
				return
			}
			//log.Printf("Packet Received from %d: dest %s, len %d\n", c.id, ipPkt.Dest.String(), len(ipPkt.Raw))
			ipPacketSink <- &InBoundIPPacket{packet: &ipPkt, clientID: c.id}
		}
	}
}

func (c *ServerConn) queueIP(pkt *OutBoundIPPacket) {
	select {
	case c.outBoundIPPacket <- pkt:
	default:
		log.Printf("Warning: Dropping packets for %s as outbound msg queue is full.\n", c.remoteAddressStr())
	}
}

func (c *ServerConn) remoteAddressStr() string {
	if len(c.remoteAddrs) == 0 {
		return ""
	}
	return c.remoteAddrs[0].String()
}

func (c *ServerConn) hadError(errInRead bool) {
	if !errInRead {
		c.conn.Close()
	}
	c.connectionOk = false
	c.server.removeClientConn(c.id)
}
