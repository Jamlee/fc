package vpn

import (
	"encoding/gob"
	"log"
	"net"
	"sync"
	"time"

	"github.com/songgao/water"
	"github.com/songgao/water/waterutil"
)

const (
	PacketInMaxBuff  = 150
	PacketOutMaxBuff = 150

	// tun device config
	tunMtuSize        = 1500
	tunPacketBuffSize = 4096
	tunTxQueLen       = 300

	servMaxInboundPacketQueue = 400
	servPerClientPacketQueue  = 200

	// for client to sent localAddr
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

type RouterManager struct {
	RouteDeletions []routeEntries

	updateGateway bool
	newGW         string

	interfaceToClose *water.Interface
}

type routeEntries struct {
	dest net.IP
	via  net.IP
	dev  string
}

// packet read from internet in vpn server outside
type ClientInBoundIPPacket struct {
	packet   *RawIPPacket
	clientID int
}

type ClientConnsManager struct {
	clientIDByAddress map[string]int
	clients           map[int]*ServerConn
	clientsLock       sync.Mutex
}

type Server struct {
	listener        net.Listener
	addrWithNetmask string

	// packet read from device like eth0
	clientInBoundIPPackets chan *ClientInBoundIPPacket

	// tun device inbound and outbound
	tunInboundIPPackets  chan *RawIPPacket
	tunOutboundIPPackets chan *RawIPPacket
	tunInterface         *water.Interface

	rm             *RouterManager
	cm             *ClientConnsManager
	lastClientID   int
	isShuttingDown bool

	wg sync.WaitGroup
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

// network is a format string like`192.168.33.1`
func NewServer(listenHost, listenPort, addrWithNetmask, iName string) (*Server, error) {
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
		tunInterface:           tunInterface,
		addrWithNetmask:        addrWithNetmask,
		clientInBoundIPPackets: make(chan *ClientInBoundIPPacket, servMaxInboundPacketQueue),
		tunInboundIPPackets:    make(chan *RawIPPacket, PacketInMaxBuff),
		tunOutboundIPPackets:   make(chan *RawIPPacket, PacketOutMaxBuff),
		cm: &ClientConnsManager{
			clientIDByAddress: map[string]int{},
			clients:           map[int]*ServerConn{},
		},
		lastClientID: 0,
	}
	return nil, s.Init(listenHost + ":" + listenPort)
}

func (s *Server) Init(addr string) (err error) {
	s.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	if err = SetDevIP(s.tunInterface.Name(), s.addrWithNetmask, false); err != nil {
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
		case pkt := <-s.clientInBoundIPPackets:
			s.routeToClient(pkt.packet)
		case pkt := <-s.tunInboundIPPackets:
			s.routeToVpnNetWork(pkt)
		}
	}
}

func (s *Server) routeToClient(pkt *RawIPPacket) {
	if pkt.Dest.IsMulticast() {
		return
	}
	s.cm.clientsLock.Lock()
	destClientID, canRouteDirectly := s.cm.clientIDByAddress[pkt.Dest.String()]
	if canRouteDirectly {
		destClient, clientExists := s.cm.clients[destClientID]
		if clientExists {
			destClient.writeToClient(pkt)
		} else {
			log.Printf("WARN: Attempted to route packet to clientID %d, which does not exist. Dropping.\n", destClientID)
		}
	}
	s.cm.clientsLock.Unlock()
}

func (s *Server) routeToVpnNetWork(pkt *RawIPPacket) {
	if pkt.Dest.IsMulticast() {
		return
	}
	s.tunOutboundIPPackets <- pkt
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
	s.cm.clientsLock.Lock()
	defer s.cm.clientsLock.Unlock()
	c.id = s.lastClientID
	s.lastClientID++
	s.cm.clients[c.id] = c
}

func (s *Server) setAddrForClient(id int, addr net.IP) {
	s.cm.clientsLock.Lock()
	defer s.cm.clientsLock.Unlock()

	s.cm.clientIDByAddress[addr.String()] = id
}

func (s *Server) removeClientConn(id int) {
	s.cm.clientsLock.Lock()
	defer s.cm.clientsLock.Unlock()

	//delete from the clientIDByAddress map if it exists
	var toDeleteAddrs []string
	for dest, itemID := range s.cm.clientIDByAddress {
		if itemID == id {
			toDeleteAddrs = append(toDeleteAddrs, dest)
		}
	}
	for _, addr := range toDeleteAddrs {
		delete(s.cm.clientIDByAddress, addr)
	}
	delete(s.cm.clients, id)
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
	go c.readRoutine(&s.isShuttingDown, s.clientInBoundIPPackets)
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

func (c *ServerConn) readRoutine(isShuttingDown *bool, ipPacketSink chan *ClientInBoundIPPacket) {
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
			ipPacketSink <- &ClientInBoundIPPacket{packet: &ipPkt, clientID: c.id}
		}
	}
}

func (c *ServerConn) writeToClient(pkt *RawIPPacket) {
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
