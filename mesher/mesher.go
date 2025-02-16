package mesher

import (
	"encoding/binary"
	"net"
	"net/netip"
)

const (
	ServerHello byte = iota
	ServerRelayFrom
	ClientHello
	ClientRelayTo
)

type Address [18]byte

type Msg struct {
	Src  net.UDPAddr
	Type byte
	Buf  []byte
}

func EncodeAddr(b []byte, addr net.UDPAddr) {
	if len(b) < 18 {
		panic(b)
	}
	ip := addr.AddrPort().Addr().As16()
	port := addr.AddrPort().Port()
	copy(b[:16], ip[:])
	binary.BigEndian.PutUint16(b[16:], port)
}

func DecodeAddr(b []byte) net.UDPAddr {
	if len(b) < 18 {
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

func Writer(addr net.UDPAddr, conn *net.UDPConn, tx chan []byte) {
	go func() {
		for b := range tx {
			conn.Write(b)
		}
	}()
}

func WriterTo(addr net.UDPAddr, conn *net.UDPConn, tx chan []byte) {
	go func() {
		for b := range tx {
			conn.WriteToUDP(b, &addr)
		}
	}()
}

func Reader(conn *net.UDPConn) chan Msg {
	channel := make(chan Msg)
	go func() {
		for {
			buf := make([]byte, 65536)
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				break
			}
			channel <- Msg{
				*addr,
				buf[0],
				buf[1:n],
			}
		}
		close(channel)
	}()
	return channel
}
