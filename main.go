package main

import "log"

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	if err := runServerCommand(); err != nil {
		log.Fatal(err)
	}
}
