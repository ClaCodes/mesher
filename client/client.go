package main

import (
    "log"
    "mesher/msg"
    "net"
    "time"
)

func reader(conn *net.UDPConn) chan []net.UDPAddr {
    peer_channel := make(chan []net.UDPAddr)
    go func() {
        for {
            rxbuf := make([]byte, 65536)
            n, addr, err := conn.ReadFromUDP(rxbuf)
            if err != nil {
                break;
            }
            if rxbuf[0] == msg.SERVER_HELLO {
                bufs := make([]net.UDPAddr, 0)
                i:=1
                for n-i>=18  {
                    a := msg.To_udp_addr(rxbuf[i:i+18])
                    i+=18
                    bufs = append(bufs, a)
                }
                log.Println("SERVER_HELLO from ", addr, ": ", bufs)
                peer_channel<-bufs
            }
        }
        log.Println("reader ending")
    }()
    return peer_channel
}

func channel_reader(peer_channel chan []net.UDPAddr) {
    go func() {
        for {
            new_peers := <-peer_channel
            for _,p := range new_peers {
                log.Println("New Peer:", p)
            }
        }
    }()
}

func main() {

    server_addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:8981")
    if err != nil {
        log.Fatal(err)
    }

    local_addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
    if err != nil {
        log.Fatal(err)
    }

    conn, err := net.DialUDP("udp", local_addr, server_addr)
    if err != nil {
        log.Fatal(err)
    }

    peer_channel := reader(conn)
    channel_reader(peer_channel)

    for {
        var txbuf [19]byte
        txbuf[0] = msg.CLIENT_HELLO
        msg.From_addr(txbuf[1:], *server_addr)
        _, err := conn.Write(txbuf[:1])
        if err != nil {
            log.Println(txbuf, err)
        }
        time.Sleep(time.Second * 1)
        txbuf[0] = msg.CLIENT_RELAY_TO
        txbuf[1] = 'a'
        _, err = conn.Write(txbuf[:])
        if err != nil {
            log.Println(txbuf, err)
        }
        time.Sleep(time.Second * 1)
    }

    conn.Close()

}


