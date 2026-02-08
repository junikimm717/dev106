package main

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/junikimm717/dev106/internal/shared"
	"github.com/junikimm717/dev106/internal/container"
)

var (
	UID          int
	GID          int
	CHOWN        string
	CHOWNEXCLUDE string
)

func initEnvVars() error {
	err := errors.New("")
	UID, err = strconv.Atoi(os.Getenv("DEV_UID"))
	if err != nil || UID < 0 {
		return errors.New("You need to set DEV_UID to some nonnegative integer!")
	}
	GID, err = strconv.Atoi(os.Getenv("DEV_GID"))
	if err != nil || GID < 0 {
		return errors.New("You need to set DEV_GID to some nonnegative integer!")
	}
	// I don't know why I have to do this for the scopes to work but ok
	CHOWN = os.Getenv("DEV_CHOWN")
	CHOWNEXCLUDE = os.Getenv("DEV_CHOWNEXCLUDE")
	return nil
}

func chown() {
	err := container.ChownDirs(
		append(strings.Split(CHOWN, ":"), shared.CONTAINER_HOME),
		strings.Split(CHOWNEXCLUDE, ":"),
		UID,
		GID,
	)
	if err != nil {
		log.Println(err)
	}
}

func writeEtc() {
	etc, err := container.ReadEtc()
	if err != nil {
		log.Println("While reading etc: " + err.Error())
		return
	}
	os.MkdirAll(shared.CONTAINER_HOME, 0o755)
	log.Printf("Creating dev106 user with %d:%d and home %s\n", UID, GID, shared.CONTAINER_HOME)
	etc.SetUIDGID(UID, GID, shared.CONTAINER_HOME)
	err = etc.Writeback()
	if err != nil {
		log.Println("While writing back to /etc: " + err.Error())
		return
	}
}

func initLoop() {
	sigCh := make(chan os.Signal, 8)
	signal.Notify(sigCh)

	go func() {
		for {
			var status syscall.WaitStatus
			pid, _ := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
			if pid <= 0 {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	for range sigCh {
	}
}

func execCommand(args []string) error {
	path, err := exec.LookPath(args[0])
	if err != nil {
		return err
	}

	return syscall.Exec(path, args, os.Environ())
}

func main() {
	err := initEnvVars()
	if err != nil {
		log.Println(err)
		log.Println("Skipping etc overwriting and chown...")
	} else {
		log.Println("Writing to etc")
		writeEtc()
		log.Println("Chowning directories in", CHOWN)
		chown()
	}
	if len(os.Args) < 2 {
		log.Print("Going on standard init loop...")
		initLoop()
	} else {
		execCommand(os.Args[1:])
	}
}
