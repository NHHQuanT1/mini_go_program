package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type MonitorConfig struct {
	MonitorProcess bool `json:"monitor_process"`
	MonitorPort    bool `json:"monitor_port"`
}

type SystemBaseline struct {
	KnownProcess map[string]bool `json:"known_process"`
	KnownPort    map[string]bool `json:"known_port"`
}

var (
	config   MonitorConfig
	baseline SystemBaseline
)

func loadConfig(configPath string) error {
	file, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config file failed: %v", err)
	}
	if err := json.Unmarshal(file, &config); err != nil {
		return fmt.Errorf("parse config file failed: %v", err)
	}
	return nil
}

func loadBaseline() error {
	if _, err := os.Stat("baseline.json"); os.IsNotExist(err) { // neu baseline.json chua co thi tao struct gom cac map rong
		baseline = SystemBaseline{
			KnownProcess: make(map[string]bool),
			KnownPort:    make(map[string]bool),
		}
		return nil
	}
	file, err := os.ReadFile("baseline.json")
	if err != nil {
		return fmt.Errorf("read config file failed: %v", err)
	}
	if err := json.Unmarshal(file, &baseline); err != nil {
		return fmt.Errorf("parse config file failed: %v", err)
	}
	return nil
}

func saveBaseline() error {
	data, err := json.MarshalIndent(baseline, "", " ")
	if err != nil {
		return fmt.Errorf("marshal baseline failed: %v", err)
	}
	if err := os.WriteFile("baseline.json", data, 0644); err != nil {
		return fmt.Errorf("write baseline failed: %v", err)
	}
	return nil
}

func promptApproval(itemType, itemName string) bool {
	fmt.Printf("Detect new %s %s \n", itemType, itemName)
	fmt.Println("Approval? (y/n): ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}
	response := strings.ToLower(scanner.Text())
	return strings.ToLower(response) == "y"
}

func getListenPorts() ([]string, error) {
	var ports []string
	for _, proto := range []string{"udp", "tcp"} {
		addresses, err := net.Interfaces()
		if err != nil {
			return nil, fmt.Errorf("get interfaces failed: %v", err)
		}
		for _, address := range addresses {
			if (address.Flags & net.FlagUp) == 0 {
				continue
			}
			conns, err := address.Addrs()
			if err != nil {
				continue
			}

			for _, conn := range conns {
				var ip net.IP
				switch v := conn.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}

				if ip == nil || ip.IsLoopback() {
					continue
				}

				for port := 1; port <= 65535; port++ {
					addr := fmt.Sprintf("%s:%d", ip.String(), port)
					conn, err := net.Dial(proto, addr)
					if err == nil {
						ports = append(ports, fmt.Sprintf("%s%s", proto, strconv.Itoa(port)))
						conn.Close()
					}
				}
			}
		}

	}
	if len(ports) == 0 {
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("netstat", "-ano")
		} else {
			cmd = exec.Command("netstat", "-tuln")
		}
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("get netstat failed: %v", err)
		}

		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Listen") {
				fields := strings.Fields(line)
				if len(fields) >= 4 {
					addr := fields[3]
					if strings.Contains(addr, ":") {
						parts := strings.Split(addr, ":")
						if len(parts) > 1 {
							port := parts[len(parts)-1]
							proto := parts[0]
							ports = append(ports, fmt.Sprintf("%s:%s", proto, port))
						}
					}
				}
			}
		}
	}
	return ports, nil
}

func checkPorts() {
	if !config.MonitorProcess {
		return
	}
	fmt.Println("Checking ports...")
	newPortsFound := false

	currentPorts, err := getListenPorts()
	if err != nil {
		fmt.Printf("get listening ports failed: %v", err)
		return
	}

	for _, port := range currentPorts {
		if _, exist := baseline.KnownPort[port]; !exist {
			newPortsFound = true
			if promptApproval("Port", port) {
				baseline.KnownPort[port] = true
				if err := saveBaseline(); err != nil {
					fmt.Printf("save baseline failed: %v", err)
				} else {
					fmt.Printf("Port approved and added to baseline: %s\n", port)
				}

			} else {
				fmt.Printf("Port %s already in baseline: %s\n", port, baseline.KnownPort[port])
			}
		}
	}
	if !newPortsFound {
		fmt.Printf("No new ports found\n")
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: ./program <config_file>")
		os.Exit(1)
	}

	if err := loadConfig(os.Args[1]); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if err := loadBaseline(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Print("\nFile monitoring program has started\n")

	checkPorts()
}
