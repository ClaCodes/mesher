package mesher

import (
	"bytes"
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
	from   netip.AddrPort
	buffer []byte
}

type response struct {
	to netip.AddrPort
	m  interface{}
}

func watchdog(addr netip.AddrPort, timeout chan netip.AddrPort) chan struct{} {
	channel := make(chan struct{})
	go func() {
		for {
			select {
			case <-channel:
			case <-time.After(5 * time.Second):
				log.Println("watchdog timeout", addr)
				timeout <- addr
				return
			}
		}
	}()
	return channel
}

func reader(conn *net.UDPConn) chan request {
	requests := make(chan request)
	go func() {
		for {
			buf := make([]byte, 65536)
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				break
			}
			sourceIP, ok := netip.AddrFromSlice(from.IP.To16())
			if !ok {
				continue
			}
			source := netip.AddrPortFrom(sourceIP, uint16(from.Port))
			requests <- request{source, buf[:n]}
		}
		log.Println("reader shutting down, closing 'requests'-channel")
		close(requests)
	}()
	return requests
}

func writer(conn *net.UDPConn, out chan response) chan struct{} {
	done := make(chan struct{})
	go func() {
		for m := range out {
			var b bytes.Buffer
			enc := gob.NewEncoder(&b)
			err := enc.Encode(&m.m)
			if err != nil {
				log.Fatal("encode:", err)
			}
			destination := net.UDPAddrFromAddrPort(m.to)
			conn.WriteToUDP(b.Bytes(), destination)
		}
		log.Println("writer shutting down, sending 'done'-signal, closing 'done'-channel")
		done <- struct{}{}
		close(done)
	}()
	return done
}

func watcher(seen chan netip.AddrPort) chan netip.AddrPort {
	timeout := make(chan netip.AddrPort)
	go func() {
		peers := make(map[netip.AddrPort]chan struct{})
		timeoutInner := make(chan netip.AddrPort)
		for seen != nil || len(peers) > 0 {
			select {
			case m, ok := <-seen:
				if !ok {
					seen = nil
					log.Println("'seen'-channel closed. Await all timeouts")
					continue
				}
				feed, ok := peers[m]
				if !ok {
					feed = watchdog(m, timeoutInner)
					peers[m] = feed
				}
				feed <- struct{}{}
			case a := <-timeoutInner:
				log.Println("watcher timeout", a)
				delete(peers, a)
				timeout <- a
			}
		}
		log.Println("watcher shutting down, closing 'timeout'-channel")
		close(timeoutInner)
		close(timeout)
	}()
	return timeout
}

/******************************************************************************/
/* SERVER                                                                     */
/******************************************************************************/

type server struct {
	peers map[netip.AddrPort]struct{}
}

type serverRequest interface {
	updateServer(s *server, from netip.AddrPort, replies chan response)
}

type getPeerList struct{}

func (m getPeerList) updateServer(s *server, from netip.AddrPort,
	replies chan response) {
	log.Println("getPeerList from", from)
	a := from
	s.peers[a] = struct{}{}
	reply := peerList{make([]netip.AddrPort, 0)}
	for k, _ := range s.peers {
		if k != a {
			reply.Addresses = append(reply.Addresses, k)
		}
	}
	replies <- response{from, reply}
}

type dataRelayTo struct {
	To   netip.AddrPort
	Data []byte
}

func (m dataRelayTo) updateServer(s *server, from netip.AddrPort,
	replies chan response) {
	log.Println("dataRelayTo from", from, "to", m.To)
	_, ok := s.peers[m.To]
	if ok {
		reply := dataRelayedFrom{
			From: from,
			Data: m.Data,
		}
		replies <- response{m.To, reply}
	}
}

func meshServer(requests chan request) chan response {
	responses := make(chan response)
	go func() {
		seen := make(chan netip.AddrPort)
		timeout := watcher(seen)
		s := server{make(map[netip.AddrPort]struct{})}
		for timeout != nil || requests != nil {
			select {
			case a, ok := <-timeout:
				if !ok {
					timeout = nil
					log.Println("'timeout'-channel closed")
					continue
				}
				delete(s.peers, a)
			case request, ok := <-requests:
				if !ok {
					requests = nil
					log.Println("'requests'-channel closed. Closing 'seen'-channel")
					close(seen)
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
		log.Println("meshServer shutting down, closing 'responses'-channel")
		close(responses)
	}()
	return responses
}

/******************************************************************************/
/* PEER                                                                       */
/******************************************************************************/

type peer struct {
	peerIds       map[netip.AddrPort]int
	nextPeerId    int
	alivePeers    map[netip.AddrPort]struct{}
	seenPeerAlive chan netip.AddrPort
}

type peerRequest interface {
	updatePeer(s *peer, from netip.AddrPort, replies chan response,
		data chan PeerMsg)
}

type peerList struct{ Addresses []netip.AddrPort }

func (m peerList) updatePeer(p *peer, from netip.AddrPort, replies chan response,
	data chan PeerMsg) {
	knownPeerIds := make(map[netip.AddrPort]int)
	for _, a := range m.Addresses {
		id, ok := p.peerIds[a]
		if !ok {
			id = p.nextPeerId
			p.nextPeerId += 1
		}
		knownPeerIds[a] = id
	}
	p.peerIds = knownPeerIds
}

type keepAlive struct{}

func (m keepAlive) updatePeer(p *peer, from netip.AddrPort, replies chan response,
	data chan PeerMsg) {
	replies <- response{from, isAlive{}}
}

type isAlive struct{}

func (m isAlive) updatePeer(p *peer, from netip.AddrPort, replies chan response,
	data chan PeerMsg) {
        log.Println("isAlive", from)
	p.alivePeers[from] = struct{}{}
	p.seenPeerAlive <- from
}

type dataRelayedFrom struct {
	From netip.AddrPort
	Data []byte
}

func (m dataRelayedFrom) updatePeer(p *peer, from netip.AddrPort,
	replies chan response, data chan PeerMsg) {
	id, ok := p.peerIds[m.From]
	if !ok {
		log.Println("dataRelayedFrom unknown Peer, ignoring it", from)
	} else {
		data <- PeerMsg{id, m.Data}
	}
}

type dataDirect struct {
	Data []byte
}

func (m dataDirect) updatePeer(p *peer, from netip.AddrPort,
	replies chan response, data chan PeerMsg) {
	log.Println("dataDirect from", from)
	id, ok := p.peerIds[from]
	if !ok {
		log.Println("dataDirect from unknown Peer, ignoring it", from)
	} else {
		data <- PeerMsg{id, m.Data}
	}
}

func meshPeer(serverAddressUdp netip.AddrPort, requests chan request,
	broadcast chan []byte) (chan PeerMsg, chan response) {
	data := make(chan PeerMsg)
	responses := make(chan response)
	go func() {
		p := peer{
			make(map[netip.AddrPort]int),
			0,
			make(map[netip.AddrPort]struct{}),
			make(chan netip.AddrPort),
		}
		timeout := watcher(p.seenPeerAlive)
		ticker := time.Tick(3 * time.Second)
		for timeout != nil || requests != nil {
			select {
			case <-ticker:
				// TODO: timout on the peer list?
				responses <- response{serverAddressUdp, getPeerList{}}
				for addr, _ := range p.peerIds {
					log.Println("Sending keepAlive", addr)
					responses <- response{addr, keepAlive{}}
				}
			case a, ok := <-timeout:
				if !ok {
					timeout = nil
					log.Println("'timeout'-channel closed")
					continue
				}
				log.Println("Peer timed out", a)
				delete(p.alivePeers, a)
			case buf, ok := <-broadcast:
				if !ok {
					log.Println("broadcast channel was closed, only reading from now on")
					broadcast = nil
					continue
				}
				for addr, _ := range p.peerIds {
					cp := make([]byte, len(buf))
					copy(cp, buf)
					_, isAlive := p.alivePeers[addr]
					if isAlive {
						m := response{
							addr,
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
					requests = nil
					log.Println("'requests'-channel closed. Closing 'p.seenPeerAlive'-channel")
					close(p.seenPeerAlive)
					continue
				}
				buf := bytes.NewBuffer(request.buffer)
				dec := gob.NewDecoder(buf)
				var m peerRequest
				err := dec.Decode(&m)
				if err != nil {
					log.Println("ignoring", err, request)
					continue
				}
				m.updatePeer(&p, request.from, responses, data)
			}
		}
		log.Println("meshPeer shutting down, closing 'responses'-channel, closing 'data'-channel")
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
		log.Println("All goroutines done, closing connection, sending 'done'-signal, closing 'done'-channel")
		conn.Close()
		done <- struct{}{}
		close(done)
	}()
	return done
}

func Peer(localAddress, serverAddress string) (chan []byte, chan struct{}, chan PeerMsg) {
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
	incoming, out := meshPeer(serverAddressUdp.AddrPort(), request, broadcast)
	innerDone := writer(conn, out)

	go func() {
		<-innerDone
		conn.Close()
		done <- struct{}{}
	}()
	return broadcast, done, incoming
}
