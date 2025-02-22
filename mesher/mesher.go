package mesher

import (
	"encoding/binary"
	"log"
	"net"
	"net/netip"
	"time"
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

type client struct {
	watchdog chan struct{}
	tx       chan []byte
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

func watchdog(addr net.UDPAddr, timeout chan net.UDPAddr) chan struct{} {
	channel := make(chan struct{})
	go func() {
		for {
			select {
			case <-channel:
			case <-time.After(5 * time.Second):
				timeout <- addr
				return
			}
		}
	}()
	return channel
}

func Server(address string) chan struct{} {
	done := make(chan struct{})
	go func() {
		localAddr, err := net.ResolveUDPAddr("udp", address)
		if err != nil {
			log.Fatal(err)
		}
		conn, err := net.ListenUDP("udp", localAddr)
		if err != nil {
			log.Fatal(err)
		}
		defer conn.Close()

		messages := Reader(conn)

		timeout := make(chan net.UDPAddr)
		clients := make(map[Address]client)
		ticker := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-ticker.C:
				for addr, c := range clients {
					buf := make([]byte, 0)
					buf = append(buf, ServerHello)
					for k, _ := range clients {
						if k != addr {
							buf = append(buf, k[:]...)
						}
					}
					log.Println("Sent     ServerHello to  ", DecodeAddr(addr[:]), ":", buf[1:])
					c.tx <- buf
				}
			case a := <-timeout:
				var m Address
				EncodeAddr(m[:], a)
				c, ok := clients[m]
				if ok {
					close(c.tx)
					delete(clients, m)
				}
				log.Println("Timed out ", a)
			case msg := <-messages:
				switch msg.Type {
				case ClientHello:
					log.Println("Received ClientHello from", msg.Src)
					var m Address
					EncodeAddr(m[:], msg.Src)
					c, ok := clients[m]
					if !ok {
						tx := make(chan []byte)
						WriterTo(msg.Src, conn, tx)
						c = client{
							watchdog(msg.Src, timeout),
							tx,
						}
						clients[m] = c
					}
					c.watchdog <- struct{}{}
				case ClientRelayTo:
					log.Println("Received ClientRelayTo from", msg.Src)
					if len(msg.Buf) >= 18 {
						var m Address
						dst := DecodeAddr(msg.Buf[:18])
						EncodeAddr(m[:], dst)
						c, ok := clients[m]
						if !ok {
							log.Println("Relaying from", msg.Src, "to", dst, "but client not present")
						} else {
							buf := make([]byte, 65536)
							buf[0] = ServerRelayFrom
							EncodeAddr(buf[1:19], msg.Src)
							copy(buf[19:], msg.Buf[18:])
							c.tx <- buf[:len(msg.Buf)+1]
						}
					}
				}
			}
		}
		done <- struct{}{}
	}()
	return done
}

func Bonder(address string) chan []byte {
	broadcast := make(chan []byte)
	serverAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		log.Fatal(err)
	}

	localAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.DialUDP("udp", localAddr, serverAddr)
	if err != nil {
		log.Fatal(err)
	}

	tx := make(chan []byte)
	Writer(*serverAddr, conn, tx)
	messages := Reader(conn)
	go func() {
		defer conn.Close()
		var peers []net.UDPAddr
		helloTimeout := time.NewTicker(3 * time.Second)
		for {
			select {
			case <-helloTimeout.C:
				var txbuf [1]byte
				txbuf[0] = ClientHello
				tx <- txbuf[:]
			case buf := <-broadcast:
				for _, p := range peers {
					txbuf := make([]byte, len(buf)+19)
					txbuf[0] = ClientRelayTo
					EncodeAddr(txbuf[1:19], p)
					copy(txbuf[19:], buf)
					tx <- txbuf[:]
				}
			case msg := <-messages:
				switch msg.Type {
				case ServerHello:
					log.Println("Received ServerHello from", msg.Src)
					bufs := make([]net.UDPAddr, 0)
					n := len(msg.Buf)
					i := 0
					for n-i >= 18 {
						a := DecodeAddr(msg.Buf[i : i+18])
						i += 18
						bufs = append(bufs, a)
					}
					peers = bufs
				case ServerRelayFrom:
					if len(msg.Buf) >= 18 {
						from := DecodeAddr(msg.Buf[:18])
						log.Println("Received ServerRelayFrom from", from)
						log.Println("Relayed data:", string(msg.Buf[18:]))
					}
				}
			}
		}
	}()
	return broadcast
}
