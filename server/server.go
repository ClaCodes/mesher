package main

import (
    "log"
    // "mesher/msg"
    "net"
)

func main() {
    port := ":8981"
    local_addr, err := net.ResolveUDPAddr("udp", port)
    if err != nil {
        log.Fatal(err)
    }
    conn, err := net.ListenUDP("udp", local_addr)
    if err != nil {
        log.Fatal(err)
    }
    buf := make([]byte, 65536)

    for {
        n, addr, err := conn.ReadFromUDP(buf)
        if err != nil {
            log.Println("Error: ", err)
            continue
        }
        log.Println("Received ", string(buf[0:n]), " from ", addr)
        // rx <- buf[0:n]
        conn.WriteToUDP(buf[0:n], addr)
    }
    conn.Close()
}
