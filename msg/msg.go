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
)

type Mesher_addres [18]byte

type Mesher_msg struct {
    Src net.UDPAddr
    Msg_type byte
    Buf []byte
}

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

func Writer(addr net.UDPAddr, conn *net.UDPConn, tx chan []byte) {
    go func() {
        for b := range tx {
            conn.Write(b)
        }
    }()
}

func Writer_to(addr net.UDPAddr, conn *net.UDPConn, tx chan []byte) {
    go func() {
        for b := range tx {
            conn.WriteToUDP(b, &addr)
        }
    }()
}

func Reader(conn *net.UDPConn) (chan Mesher_msg) {
    client_msg_channel := make(chan Mesher_msg)
    go func() {
        for {
            buf := make([]byte, 65536)
            n, addr, err := conn.ReadFromUDP(buf)
            if err != nil {
                break;
            }
            client_msg_channel <- Mesher_msg {
                *addr,
                buf[0],
                buf[1:n],
            }
        }
        close(client_msg_channel)
    }()
    return client_msg_channel
}
