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
	PacketInMaxBuff           = 150
	PacketOutMaxBuff          = 150
	tunMtuSize                = 1500
	tunPacketBuffSize         = 4096
	tunTxQueLen               = 300
	servMaxInboundPacketQueue = 400
	servPerClientPacketQueue  = 200

	// for connection
	PacketUnknown PacketType = iota
	PacketIP
	PacketLocalAddr
)

type PacketType byte

// packet represention
type RawIPPacket struct {
	Raw      []byte
	Dest     net.IP
	Protocol waterutil.IPProtocol
}

// packet read from internet in server side
type NetInBoundIPPacket struct {
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

	// packet read from device like eth0
	netInboundIPPackets chan *NetInBoundIPPacket

	// tun device inbound and outbound
	tunInboundIPPackets  chan *RawIPPacket
	tunOutboundIPPackets chan *RawIPPacket

	tunInterface *water.Interface
	rm           RouterManager
	wg           sync.WaitGroup
	lastClientID int
}

type ServerConn struct {
	id               int
	conn             net.Conn
	outBoundIPPacket chan *RawIPPacket
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
		tunInterface:         tunInterface,
		localAddr:            netIP,
		localNetMask:         localNetMask,
		netInboundIPPackets:  make(chan *NetInBoundIPPacket, servMaxInboundPacketQueue),
		tunInboundIPPackets:  make(chan *RawIPPacket, PacketInMaxBuff),
		tunOutboundIPPackets: make(chan *RawIPPacket, PacketOutMaxBuff),
		clientIDByAddress:    map[string]int{},
		clients:              map[int]*ServerConn{},
		lastClientID:         0,
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

func (s *Server) Run() {
	go s.acceptRoutine()
	go s.dispatchRoutine()
	go tunWriteRoutine(s.tunInterface, s.tunOutboundIPPackets, &s.wg, &s.isShuttingDown)
	go tunReadRoutine(s.tunInterface, s.tunInboundIPPackets, &s.wg, &s.isShuttingDown)
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

func (s *Server) dispatchRoutine() {
	for !s.isShuttingDown {
		select {
		case pkt := <-s.netInboundIPPackets:
			//log.Printf("Got packet from NET: %s-%d len %d\n", pkt.pkt.Dest, pkt.clientID, len(pkt.pkt.Raw))
			s.route(pkt.packet)
		case pkt := <-s.tunInboundIPPackets:
			//log.Printf("Got packet from DEV: %s len %d\n", pkt.Dest, len(pkt.Raw))
			s.route(pkt)
		}
	}
}

func (s *Server) route(pkt *RawIPPacket) {
	if pkt.Dest.IsMulticast() { //Don't forward multicast
		return
	}

	s.clientsLock.Lock()
	destClientID, canRouteDirectly := s.clientIDByAddress[pkt.Dest.String()]
	if canRouteDirectly {
		destClient, clientExists := s.clients[destClientID]
		if clientExists {
			destClient.queueIP(pkt)
			//log.Println("Routing to CLIENT")
		} else {
			log.Printf("WARN: Attempted to route packet to clientID %d, which does not exist. Dropping.\n", destClientID)
		}
	}
	s.clientsLock.Unlock()
	if !canRouteDirectly {
		s.tunOutboundIPPackets <- pkt
		//log.Println("Routing to DEV")
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
	c.outBoundIPPacket = make(chan *RawIPPacket, servPerClientPacketQueue)
	c.connectionOk = true
	c.server = s
	log.Printf("New connection from %s (%d)\n", c.conn.RemoteAddr().String(), c.id)
	go c.readRoutine(&s.isShuttingDown, s.netInboundIPPackets)
	go c.writeRoutine(&s.isShuttingDown)
}

func (c *ServerConn) writeRoutine(isShuttingDown *bool) {
	encoder := gob.NewEncoder(c.conn)
	for !*isShuttingDown && c.connectionOk {
		select {
		case pkt := <-c.outBoundIPPacket:
			encoder.Encode(PacketIP)
			err := encoder.Encode(pkt)
			if err != nil {
				log.Printf("Write error for %s: %s\n", c.conn.RemoteAddr().String(), err.Error())
				c.hadError(false)
				return
			}
		}
	}
}

func (c *ServerConn) readRoutine(isShuttingDown *bool, ipPacketSink chan *NetInBoundIPPacket) {
	decoder := gob.NewDecoder(c.conn)

	for !*isShuttingDown && c.connectionOk {
		var PacketType PacketType
		err := decoder.Decode(&PacketType)
		if err != nil {
			if !*isShuttingDown {
				log.Printf("Client read error: %s\n", err.Error())
			}
			c.hadError(true)
			return
		}

		switch PacketType {
		case PacketLocalAddr:
			var localAddr net.IP
			err := decoder.Decode(&localAddr)
			if err != nil {
				log.Printf("Could not decode net.IP: %s", err.Error())
				c.hadError(false)
				return
			}
			c.remoteAddrs = append(c.remoteAddrs, localAddr)
			c.server.setAddrForClient(c.id, localAddr)

		case PacketIP:
			var ipPkt RawIPPacket
			err := decoder.Decode(&ipPkt)
			if err != nil {
				log.Printf("Could not decode IPPacket: %s", err.Error())
				c.hadError(false)
				return
			}
			//log.Printf("Packet Received from %d: dest %s, len %d\n", c.id, ipPkt.Dest.String(), len(ipPkt.Raw))
			ipPacketSink <- &NetInBoundIPPacket{packet: &ipPkt, clientID: c.id}
		}
	}
}

func (c *ServerConn) queueIP(pkt *RawIPPacket) {
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

////////////////////////////////////////////////////////////////////////////////////////
//
//  utils
//
/////////////////////////////////////////////////////////////////////////////////////////

func tunReadRoutine(dev *water.Interface, packetsIn chan *RawIPPacket, wg *sync.WaitGroup, isShuttingDown *bool) {
	wg.Add(1)
	defer wg.Done()

	for !*isShuttingDown {
		packet := make([]byte, tunPacketBuffSize)
		n, err := dev.Read(packet)
		if err != nil {
			if !*isShuttingDown {
				log.Printf("%s read err: %s\n", dev.Name(), err.Error())
			}
			close(packetsIn)
			return
		}
		p := &RawIPPacket{
			Raw:      packet[:n],
			Dest:     waterutil.IPv4Destination(packet[:n]),
			Protocol: waterutil.IPv4Protocol(packet[:n]),
		}
		packetsIn <- p
		//log.Printf("Packet Received: dest %s, len %d\n", p.Dest.String(), len(p.Raw))
	}
}

func tunWriteRoutine(dev *water.Interface, packetsOut chan *RawIPPacket, wg *sync.WaitGroup, isShuttingDown *bool) {
	wg.Add(1)
	defer wg.Done()

	for !*isShuttingDown {
		pkt := <-packetsOut
		w, err := dev.Write(pkt.Raw)
		if err != nil {
			log.Printf("Write to %s failed: %s\n", dev.Name(), err.Error())
			return
		}
		if w != len(pkt.Raw) {
			log.Printf("WARN: Write to %s has mismatched len: %d != %d\n", dev.Name(), w, len(pkt.Raw))
		}
	}
}
