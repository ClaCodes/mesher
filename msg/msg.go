package msg

import (
    "net"
    "net/netip"
    "encoding/binary"
)

const (
    SERVER_HELLO byte = iota
    SERVER_RELAY_FROM
    CLIENT_HELLO
    CLIENT_RELAY_TO
    PEER
)

type Mesher_addres [18]byte

func From_addr(b []byte, addr net.UDPAddr) {
    if len(b) != 18 {
        panic(b)
    }
    ip :=  addr.AddrPort().Addr().As16()
    port := addr.AddrPort().Port()
    copy(b[:16], ip[:])
    binary.BigEndian.PutUint16(b[16:], port)
}

func To_udp_addr(b []byte) net.UDPAddr {
    if len(b) != 18 {
        panic(b)
    }
    var ip netip.Addr
    ip, ok := netip.AddrFromSlice(b[:16])
    if !ok {
        panic("bla")
    }
    port := binary.BigEndian.Uint16(b[16:])
    addr := netip.AddrPortFrom(ip, port)
    return *net.UDPAddrFromAddrPort(addr)
}
