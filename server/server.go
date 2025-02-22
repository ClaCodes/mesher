package main

import (
	"mesher/mesher"
)

func main() {
	address := ":8981"
	done := mesher.Server(address)
	<-done
}
