package mesher

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"log"
	"net"
	"net/netip"
	"time"
)

/******************************************************************************/
/* GENERAL                                                                    */
/******************************************************************************/

type request struct {
	from   *net.UDPAddr
	buffer []byte
}

type response struct {
	to *net.UDPAddr
	m  interface{}
}

// TODO net.UDPAddr as map-key. Alternative?
type address [18]byte

func addrKey(addr *net.UDPAddr) address {
	var a address
	ip := addr.AddrPort().Addr().As16()
	port := addr.AddrPort().Port()
	copy(a[:16], ip[:])
	binary.BigEndian.PutUint16(a[16:], port)
	return a
}

func addrFromKey(a address) *net.UDPAddr {
	var ip netip.Addr
	ip, ok := netip.AddrFromSlice(a[:16])
	if !ok {
		return nil
	}
	port := binary.BigEndian.Uint16(a[16:])
	addr := netip.AddrPortFrom(ip, port)
	return net.UDPAddrFromAddrPort(addr)
}

func watchdog(addr *net.UDPAddr, timeout chan *net.UDPAddr) chan struct{} {
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

func reader(conn *net.UDPConn) chan request {
	c := make(chan request)
	go func() {
		for {
			buf := make([]byte, 65536)
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				break
			}
			c <- request{from, buf[:n]}
		}
		log.Println("reader shutting down")
		close(c)
	}()
	return c
}

func writer(conn *net.UDPConn, out chan response) chan struct{} {
	done := make(chan struct{})
	go func() {
		for m := range out {
			if m.to == nil {
				continue
			}
			var b bytes.Buffer
			enc := gob.NewEncoder(&b)
			err := enc.Encode(&m.m)
			if err != nil {
				log.Fatal("encode:", err)
			}
			conn.WriteToUDP(b.Bytes(), m.to)
		}
		done <- struct{}{}
	}()
	return done
}

func watcher(seen chan *net.UDPAddr) chan *net.UDPAddr {
	timeout := make(chan *net.UDPAddr)
	go func() {
		peers := make(map[address]chan struct{})
		timeout := make(chan *net.UDPAddr)
		for seen != nil || timeout != nil {
			select {
			case m, ok := <-seen:
				if !ok {
					seen = nil
					continue
				}
				feed, ok := peers[addrKey(m)]
				if !ok {
					feed = watchdog(m, timeout)
					peers[addrKey(m)] = feed
				}
				feed <- struct{}{}
			case a, ok := <-timeout:
				if !ok {
					timeout = nil
					continue
				}
				delete(peers, addrKey(a))
				timeout <- a
			}
		}
		close(timeout)
	}()
	return timeout
}

/******************************************************************************/
/* SERVER                                                                     */
/******************************************************************************/

type server struct {
	peers map[address]struct{}
}

type serverRequest interface {
	updateServer(s *server, from *net.UDPAddr, replies chan response)
}

type getPeerList struct{}

func (m getPeerList) updateServer(s *server, from *net.UDPAddr,
	replies chan response) {
	a := addrKey(from)
	s.peers[a] = struct{}{}
	reply := peerList{make([]address, 0)}
	for k, _ := range s.peers {
		if k != a {
			reply.Peers = append(reply.Peers, k)
		}
	}
	replies <- response{from, reply}
}

type dataRelayTo struct {
	To   address
	Data []byte
}

func (m dataRelayTo) updateServer(s *server, from *net.UDPAddr,
	replies chan response) {
	log.Println("relaying from", from, "to", m.To)
	_, ok := s.peers[m.To]
	if ok {
		reply := dataRelayedFrom{
			From: addrKey(from),
			Data: m.Data,
		}
		replies <- response{addrFromKey(m.To), reply}
	}
}

func meshServer(requests chan request) chan response {
	responses := make(chan response)
	go func() {
		seen := make(chan *net.UDPAddr)
		timeout := watcher(seen)
		s := server{make(map[address]struct{})}
		for timeout != nil || requests != nil {
			select {
			case a, ok := <-timeout:
				if !ok {
					timeout = nil
					continue
				}
				delete(s.peers, addrKey(a))
			case request, ok := <-requests:
				if !ok {
					requests = nil
					continue
				}
				buf := bytes.NewBuffer(request.buffer)
				dec := gob.NewDecoder(buf)
				var m serverRequest
				err := dec.Decode(&m)
				if err != nil {
					log.Println("ignoring", err, request)
					continue
				}
				seen <- request.from
				m.updateServer(&s, request.from, responses)
			}
		}
		close(responses)
	}()
	return responses
}

/******************************************************************************/
/* PEER                                                                       */
/******************************************************************************/

type peer struct {
	peerIds    map[address]int
	openPeers  map[address]struct{}
	nextPeerId int
}

type peerRequest interface {
	updatePeer(s *peer, from *net.UDPAddr, replies chan response,
		data chan PeerMsg)
}

type peerList struct{ Peers []address }

func (m peerList) updatePeer(b *peer, from *net.UDPAddr, replies chan response,
	data chan PeerMsg) {
	for _, p := range m.Peers {
		b.peerIds[p] = b.nextPeerId
		b.nextPeerId += 1
	}
}

type keepAlive struct{}

func (m keepAlive) updatePeer(b *peer, from *net.UDPAddr, replies chan response,
	data chan PeerMsg) {
	replies <- response{from, isAlive{}}
}

type isAlive struct{}

func (m isAlive) updatePeer(b *peer, from *net.UDPAddr, replies chan response,
	data chan PeerMsg) {
	b.openPeers[addrKey(from)] = struct{}{}
	// openAddresses <- m.src
}

type dataRelayedFrom struct {
	From address
	Data []byte
}

func (m dataRelayedFrom) updatePeer(b *peer, from *net.UDPAddr,
	replies chan response, data chan PeerMsg) {
	id, ok := b.peerIds[m.From]
	if !ok {
		b.peerIds[m.From] = b.nextPeerId
		id = b.nextPeerId
		b.nextPeerId += 1
	}
	data <- PeerMsg{id, m.Data}
}

type dataDirect struct {
	Data []byte
}

func (m dataDirect) updatePeer(b *peer, from *net.UDPAddr,
	replies chan response, data chan PeerMsg) {
	log.Println("dataDirect from", from)
	a := addrKey(from)
	id, ok := b.peerIds[a]
	if !ok {
		b.peerIds[a] = b.nextPeerId
		id = b.nextPeerId
		b.nextPeerId += 1
	}
	data <- PeerMsg{id, m.Data}
}

func meshPeer(serverAddressUdp *net.UDPAddr, requests chan request,
	broadcast chan []byte) (chan PeerMsg, chan response) {
	data := make(chan PeerMsg)
	responses := make(chan response)
	go func() {
		seen := make(chan *net.UDPAddr)
		timeout := watcher(seen)
		ticker := time.Tick(3 * time.Second)
		p := peer{make(map[address]int), make(map[address]struct{}), 0}
	pollLoop:
		for {
			select {
			case <-ticker:
				responses <- response{serverAddressUdp, getPeerList{}}
				for addr, _ := range p.peerIds {
					log.Println("Sending keep alive")
					responses <- response{addrFromKey(addr), keepAlive{}}
				}
			case a := <-timeout:
				log.Println("Timed responses", a)
				delete(p.peerIds, addrKey(a))
				delete(p.openPeers, addrKey(a))
			case buf := <-broadcast:
				/*TODO what if broadcast is closed */
				for addr, _ := range p.peerIds {
					cp := make([]byte, len(buf))
					copy(cp, buf)
					_, isAlive := p.openPeers[addr]
					if isAlive {
						m := response{
							addrFromKey(addr),
							dataDirect{cp},
						}
						responses <- m
					} else {
						m := response{
							serverAddressUdp,
							dataRelayTo{addr, cp},
						}
						responses <- m
					}
				}
			case request, ok := <-requests:
				if !ok {
					break pollLoop
				}
				buf := bytes.NewBuffer(request.buffer)
				dec := gob.NewDecoder(buf)
				var m peerRequest
				err := dec.Decode(&m)
				if err != nil {
					log.Println("ignoring", err, request)
					continue
				}
				seen <- request.from
				m.updatePeer(&p, request.from, responses, data)
			}
		}
		close(data)
		close(responses)
	}()
	return data, responses
}

/******************************************************************************/
/* PUBLIC                                                                     */
/******************************************************************************/

type PeerMsg struct {
	PeerId int
	Buf    []byte
}

func Server(serverAddress string) chan struct{} {
	gob.Register(getPeerList{})
	gob.Register(peerList{})
	gob.Register(keepAlive{})
	gob.Register(isAlive{})
	gob.Register(dataRelayTo{})
	gob.Register(dataRelayedFrom{})
	gob.Register(dataDirect{})

	serverAddressUDP, err := net.ResolveUDPAddr("udp", serverAddress)
	if err != nil {
		log.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", serverAddressUDP)
	if err != nil {
		log.Fatal(err)
	}

	request := reader(conn)
	out := meshServer(request)
	innerDone := writer(conn, out)

	done := make(chan struct{})
	go func() {
		<-innerDone
		conn.Close()
		done <- struct{}{}
	}()
	return done
}

func Bonder(localAddress, serverAddress string) (chan []byte, chan struct{}, chan PeerMsg) {
	gob.Register(getPeerList{})
	gob.Register(peerList{})
	gob.Register(keepAlive{})
	gob.Register(isAlive{})
	gob.Register(dataRelayTo{})
	gob.Register(dataRelayedFrom{})
	gob.Register(dataDirect{})

	serverAddressUdp, err := net.ResolveUDPAddr("udp", serverAddress)
	if err != nil {
		log.Fatal(err)
	}

	localAddressUDP, err := net.ResolveUDPAddr("udp", localAddress)
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.ListenUDP("udp", localAddressUDP)
	if err != nil {
		log.Fatal(err)
	}

	done := make(chan struct{})
	broadcast := make(chan []byte)

	request := reader(conn)
	incoming, out := meshPeer(serverAddressUdp, request, broadcast)
	innerDone := writer(conn, out)

	go func() {
		<-innerDone
		conn.Close()
		done <- struct{}{}
	}()
	return broadcast, done, incoming
}
