package main

import (
	"fmt"
	"log"
	"os/exec"
)

func main() {
	cmd := exec.Command("sleep", "1")
	log.Printf("Running command and waiting for it to finish...")
	err := cmd.Start()
	if err != nil {
		log.Fatal(err)
	}
	pid := cmd.Process.Pid
	println(pid)
	cmd2 := exec.Command("kill", fmt.Sprintf("%v", pid))
	err = cmd2.Run()
	if err != nil {
		log.Fatalf("fatal#2%v", err)
	}
	err = cmd.Wait()
	if err != nil {
		log.Fatalf("fatal#3%v", err)
	}
	log.Printf("Command finished with error: %v", err)
}
