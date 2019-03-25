package vpn

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"os"
	"strconv"

	"github.com/vishvananda/netlink"
)

func SetInterfaceStatus(iName string, up bool, debug bool) error {
	link, err := netlink.LinkByName(iName)
	if err != nil {
		if up {
			netlink.LinkSetUp(link)
		} else {
			netlink.LinkSetDown(link)
		}
	}
	return err
}

func SetDevIP(iName string, addrWithNetmask string, debug bool) error {
	addr, err := netlink.ParseAddr(addrWithNetmask)
	if err != nil {
		return err
	}
	link, err := netlink.LinkByName(iName)
	if err == nil {
		netlink.AddrAdd(link, addr)
	}
	return err
}

func SetDefaultGateway(gw, iName string, debug bool) error {
	link, err := netlink.LinkByName(iName)
	if err != nil {
		return err
	}
	dst := &net.IPNet{
		IP:   net.IPv4(0, 0, 0, 0),
		Mask: net.CIDRMask(0, 32),
	}
	route := netlink.Route{LinkIndex: link.Attrs().Index, Dst: dst}
	err = netlink.RouteAdd(&route)
	return err
}

func AddRoute(addr, viaAddr net.IP, iName string, debug bool) error {
	link, err := netlink.LinkByName(iName)
	if err != nil {
		return err
	}
	route := netlink.Route{LinkIndex: link.Attrs().Index, Src: addr, Gw: viaAddr}
	err = netlink.RouteAdd(&route)
	return err
}

func DelRoute(addr, viaAddr net.IP, iName string, debug bool) error {
	link, err := netlink.LinkByName(iName)
	if err != nil {
		return err
	}
	route := netlink.Route{LinkIndex: link.Attrs().Index, Src: addr, Gw: viaAddr}
	err = netlink.RouteDel(&route)
	return err
}

func GetNetGateway() (gw, dev string, err error) {
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return "", "", err
	}

	defer file.Close()
	rd := bufio.NewReader(file)

	s2byte := func(s string) byte {
		b, _ := strconv.ParseUint(s, 16, 8)
		return byte(b)
	}

	for {
		line, isPrefix, err := rd.ReadLine()

		if err != nil {
			return "", "", err
		}
		if isPrefix {
			return "", "", errors.New("Parse error: Line too long")
		}
		buf := bytes.NewBuffer(line)
		scanner := bufio.NewScanner(buf)
		scanner.Split(bufio.ScanWords)
		tokens := make([]string, 0, 8)

		for scanner.Scan() {
			tokens = append(tokens, scanner.Text())
		}

		iface := tokens[0]
		dest := tokens[1]
		gw := tokens[2]
		mask := tokens[7]

		if bytes.Equal([]byte(dest), []byte("00000000")) &&
			bytes.Equal([]byte(mask), []byte("00000000")) {
			a := s2byte(gw[6:8])
			b := s2byte(gw[4:6])
			c := s2byte(gw[2:4])
			d := s2byte(gw[0:2])

			ip := net.IPv4(a, b, c, d)

			return ip.String(), iface, nil
		}
	}
}
