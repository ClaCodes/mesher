package main

import (
    "log"
    "mesher/msg"
    "net"
    "time"
)

func client(server_msg_channel chan msg.Mesher_msg, tx chan []byte) {
    go func() {
        var peers []net.UDPAddr
        hello_ticker := time.NewTicker(3 * time.Second)
        peer_ticker := time.NewTicker(5 * time.Second)
        for {
            select {
            case <- hello_ticker.C:
                var txbuf [1]byte
                txbuf[0] = msg.CLIENT_HELLO
                tx <- txbuf[:]
            case <- peer_ticker.C:
                for _,p := range peers {
                    var txbuf [20]byte
                    txbuf[0] = msg.CLIENT_RELAY_TO
                    msg.From_addr(txbuf[1:19], p)
                    txbuf[19] = 'b'
                    log.Println("To Peer:", txbuf)
                    tx <- txbuf[:]
                }
            case server_msg := <- server_msg_channel:
                switch server_msg.Msg_type {
                case msg.SERVER_HELLO:
                    log.Println("Received SERVER_HELLO from", server_msg.Src)
                    bufs := make([]net.UDPAddr, 0)
                    n := len(server_msg.Buf)
                    i:=0
                    for n-i>=18  {
                        a := msg.To_udp_addr(server_msg.Buf[i:i+18])
                        i+=18
                        bufs = append(bufs, a)
                    }
                    peers = bufs
                case msg.SERVER_RELAY_FROM:
                    if len(server_msg.Buf) >= 18 {
                        relayed_from := msg.To_udp_addr(server_msg.Buf[:18])
                        log.Println("Received SERVER_RELAY_FROM from", relayed_from)
                        log.Println("Relayed data:", string(server_msg.Buf[18:]))
                    }
                }
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

    tx := make(chan []byte)
    msg.Writer(*server_addr, conn, tx)
    peer_channel := msg.Reader(conn)
    client(peer_channel, tx)

    for { }

    conn.Close()

}


