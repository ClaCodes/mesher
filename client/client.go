package main

import (
    "log"
    "encoding/binary"
    "mesher/msg"
    "net"
    "time"
)

func reader(conn *net.UDPConn) {
    go func() {
        for {
            rxbuf := make([]byte, 65536)
            n, addr, err := conn.ReadFromUDP(rxbuf)
            if err != nil {
                break;
            }
            log.Println("Received ", string(rxbuf[0:n]), " from ", addr)
        }
        log.Println("reader ending")
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

    reader(conn)

    for {
        var txbuf [4]byte
        binary.BigEndian.PutUint32(txbuf[:], msg.Client_hello)
        _, err := conn.Write(txbuf[:])
        if err != nil {
            log.Println(txbuf, err)
        }
        time.Sleep(time.Second * 1)
    }

    conn.Close()

}


