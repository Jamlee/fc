package vpn

import (
	"net"

	"github.com/songgao/water"
)

// clean the route config when exit
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
