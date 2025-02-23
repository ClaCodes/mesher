package mesher

import (
	"encoding/binary"
	"log"
	"net"
	"net/netip"
	"time"
)

const (
	serverHello byte = iota
	serverRelayFrom
	clientHello
	clientRelayTo
)

type address [18]byte

type msg struct {
	src  net.UDPAddr
	kind byte
	buf  []byte
}

type client struct {
	watchdog chan struct{}
	tx       chan []byte
}

func encodeAddr(b []byte, addr net.UDPAddr) {
	if len(b) < 18 {
		panic(b)
	}
	ip := addr.AddrPort().Addr().As16()
	port := addr.AddrPort().Port()
	copy(b[:16], ip[:])
	binary.BigEndian.PutUint16(b[16:], port)
}

func decodeAddr(b []byte) net.UDPAddr {
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

func writer(addr net.UDPAddr, conn *net.UDPConn, tx chan []byte) {
	go func() {
		for b := range tx {
			conn.Write(b)
		}
	}()
}

func writerTo(addr net.UDPAddr, conn *net.UDPConn, tx chan []byte) {
	go func() {
		for b := range tx {
			conn.WriteToUDP(b, &addr)
		}
	}()
}

func reader(conn *net.UDPConn) chan msg {
	channel := make(chan msg)
	go func() {
		for {
			buf := make([]byte, 65536)
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				break
			}
			channel <- msg{
				*addr,
				buf[0],
				buf[1:n],
			}
		}
		log.Println("reader shutting down");
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

type PeerMsg struct {
	PeerId int
	Buf []byte
}

func Server(serverAddress string) chan struct{} {
	done := make(chan struct{})
	go func() {
		localAddr, err := net.ResolveUDPAddr("udp", serverAddress)
		if err != nil {
			log.Fatal(err)
		}
		conn, err := net.ListenUDP("udp", localAddr)
		if err != nil {
			log.Fatal(err)
		}
		defer conn.Close()

		messages := reader(conn)

		timeout := make(chan net.UDPAddr)
		clients := make(map[address]client)
		ticker := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-ticker.C:
				for addr, c := range clients {
					buf := make([]byte, 0)
					buf = append(buf, serverHello)
					for k, _ := range clients {
						if k != addr {
							buf = append(buf, k[:]...)
						}
					}
					c.tx <- buf
				}
			case a := <-timeout:
				var m address
				encodeAddr(m[:], a)
				c, ok := clients[m]
				if ok {
					close(c.tx)
					delete(clients, m)
				}
				log.Println("Timed out ", a)
			case m := <-messages:
				switch m.kind {
				case clientHello:
					var a address
					encodeAddr(a[:], m.src)
					c, ok := clients[a]
					if !ok {
						tx := make(chan []byte)
						writerTo(m.src, conn, tx)
						c = client{
							watchdog(m.src, timeout),
							tx,
						}
						clients[a] = c
					}
					c.watchdog <- struct{}{}
				case clientRelayTo:
					if len(m.buf) >= 18 {
						var a address
						dst := decodeAddr(m.buf[:18])
						encodeAddr(a[:], dst)
						c, ok := clients[a]
						if !ok {
							log.Println("Relaying from", m.src, "to", dst, "but client not present")
						} else {
							buf := make([]byte, 65536)
							buf[0] = serverRelayFrom
							encodeAddr(buf[1:19], m.src)
							copy(buf[19:], m.buf[18:])
							c.tx <- buf[:len(m.buf)+1]
						}
					}
				}
			}
		}
		done <- struct{}{}
	}()
	return done
}

func Bonder(serverAddress string) (chan []byte, chan struct{}, chan PeerMsg) {
	done := make(chan struct{})
	broadcast := make(chan []byte)
	incoming := make(chan PeerMsg)
	serverAddr, err := net.ResolveUDPAddr("udp", serverAddress)
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
	writer(*serverAddr, conn, tx)
	messages := reader(conn)
	go func() {
		defer conn.Close()
		var peers []net.UDPAddr
		helloTimeout := time.NewTicker(3 * time.Second)
		peerIds := make(map[address]int)
		nextPeer := 0
		pollLoop: for {
			select {
			case <-helloTimeout.C:
				var txbuf [1]byte
				txbuf[0] = clientHello
				tx <- txbuf[:]
			case buf := <-broadcast:
				for _, p := range peers {
					txbuf := make([]byte, len(buf)+19)
					txbuf[0] = clientRelayTo
					encodeAddr(txbuf[1:19], p)
					copy(txbuf[19:], buf)
					tx <- txbuf[:]
				}
			case m, ok := <-messages:
				if !ok {
					break pollLoop;
				}
				switch m.kind {
				case serverHello:
					bufs := make([]net.UDPAddr, 0)
					n := len(m.buf)
					i := 0
					for n-i >= 18 {
						a := decodeAddr(m.buf[i : i+18])
						i += 18
						bufs = append(bufs, a)
					}
					peers = bufs
				case serverRelayFrom:
					if len(m.buf) >= 18 {
						from := decodeAddr(m.buf[:18])
						var key address
						encodeAddr(key[:], from)
						p, ok := peerIds[key]
						if !ok {
							p = nextPeer
							nextPeer += 1
							peerIds[key] = p
						}
						incoming<-PeerMsg{p, m.buf[18:]}
					}
				}
			}
		}
		log.Println("Bonder shutting down")
		done <- struct{}{}
	}()
	return broadcast, done, incoming
}
