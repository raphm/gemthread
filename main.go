package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var _server_url string

func server_url() string {
	return _server_url
}

var _help_path string

func help_path() string {
	return _help_path
}

var _database_path string

func database_path() string {
	return _database_path
}

var _socket_path string

func socket_path() string {
	return _socket_path
}

func main() {

	var l net.Listener
	var err error
	should_exit := false

	_config_path := "gemthread.cfg"

	flag.StringVar(&_config_path,
		"config",
		_config_path,
		"path to gemthread configuration file")

	flag.Parse()

	_server_url = ""
	_database_path = "gemthread.db"
	_help_path = "help.gmi"
	_socket_path = "scgi.sock"

	config_data, err := ioutil.ReadFile(_config_path)
	if err != nil {
		fmt.Printf("Error reading %s: %s\n", _config_path, err.Error())
		return
	}

	config_lines := strings.Split(string(config_data), "\n")

	for _, line := range config_lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			fmt.Printf("Invalid configuration line: %s\n", line)
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(parts[0])) {
		case "SERVER_URL":
			_server_url = strings.TrimSuffix(strings.TrimSpace(parts[1]), "/")
		case "HELP_PATH":
			_help_path = strings.TrimSpace(parts[1])
		case "DATABASE_PATH":
			_database_path = strings.TrimSpace(parts[1])
		case "SOCKET_PATH":
			_socket_path = strings.TrimSpace(parts[1])
		default:
			fmt.Printf("Invalid configuration line: %s\n", line)
		}
	}

	if len(_server_url) == 0 || !strings.HasPrefix(_server_url, "gemini://") {
		fmt.Printf("Unable to continue due to invalid gemthread URL: %s\n", _server_url)
		return
	}

	if len(_help_path) == 0 {
		fmt.Printf("Unable to continue due to invalid help file path: %s\n", _help_path)
		return
	}

	if len(_database_path) == 0 {
		fmt.Printf("Unable to continue due to invalid database path: %s\n", _database_path)
		return
	}

	if len(_socket_path) == 0 {
		fmt.Printf("Unable to continue due to invalid socket path: %s\n", _socket_path)
		return
	}

	// Molly Brown only supports UNIX sockets
	l, err = net.Listen("unix", socket_path())

	if err != nil {
		fmt.Println("SCGI listen error", err.Error())
		return
	}

	defer l.Close()

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("Received interrupt. Exiting.")
		should_exit = true
		l.Close()
	}()

	db, err := db_open(database_path(), false)
	if err != nil {
		fmt.Printf("Database error: %s\n", err.Error())
		return
	}

	defer db.Close()

	for {
		fd, err := l.Accept()
		if err != nil {
			if should_exit {
				return
			}
			fmt.Println("SCGI accept error", err.Error())
			continue
		}
		go handle_request(fd, db)
	}
}
