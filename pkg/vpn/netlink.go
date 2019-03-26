package vpn

import (
	"net"

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
	route := netlink.Route{LinkIndex: link.Attrs().Index, Gw: net.ParseIP(gw)}
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
