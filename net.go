package main

import (
	"log"

	"golang.org/x/sys/unix"
)

const BACKLOG int = 64

func Accept(fd int) (int, error) {
	cfd, _, err := unix.Accept(fd)
	// ignore client addr for now
	return cfd, err
}

func Read(fd int, buf []byte) (int, error) {
	return unix.Read(fd, buf)
}

func Write(fd int, buf []byte) (int, error) {
	return unix.Write(fd, buf)
}

func Connect(host [4]byte, port int) (int, error) {
	s, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		log.Printf("init socket err: %v\n", err)
		return -1, err
	}
	var addr unix.SockaddrInet4
	addr.Addr = host
	addr.Port = port
	err = unix.Connect(s, &addr)
	if err != nil {
		log.Printf("connect err: %v\n", err)
		return -1, err
	}
	return s, nil
}

func TcpServer(port int) (int, error) {
	s, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		log.Printf("init socket err: %v\n", err)
		return -1, err
	}
	/* Make sure connection-intensive things like the redis benchmark
	 * will be able to close/open sockets a zillion of times */
	err = unix.SetsockoptInt(s, unix.SOL_SOCKET, unix.SO_REUSEADDR, 0x01)
	if err != nil {
		log.Printf("set SO_REUSEADDR err: %v\n", err)
		unix.Close(s)
		return -1, err
	}
	var addr unix.SockaddrInet4
	// golang.syscall will handle htons
	addr.Port = port
	// golang will set addr.Addr = any(0)
	err = unix.Bind(s, &addr)
	if err != nil {
		log.Printf("bind addr err: %v\n", err)
		unix.Close(s)
		return -1, err
	}
	err = unix.Listen(s, BACKLOG)
	if err != nil {
		log.Printf("listen socket err: %v\n", err)
		unix.Close(s)
		return -1, err
	}
	return s, nil
}
