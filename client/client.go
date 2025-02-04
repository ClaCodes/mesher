package main

import (
    "log"
    // "mesher/msg"
    "net"
    "strconv"
    "time"
)

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

    i := 0
    for {
        msg := strconv.Itoa(i)
        i++
        txbuf := []byte(msg)
        _, err := conn.Write(txbuf)
        if err != nil {
            log.Println(msg, err)
        }
        time.Sleep(time.Second * 1)
        rxbuf := make([]byte, 65536)
        n, addr, err := conn.ReadFromUDP(rxbuf)
        log.Println("Received ", string(rxbuf[0:n]), " from ", addr)
    }

    conn.Close()

}


