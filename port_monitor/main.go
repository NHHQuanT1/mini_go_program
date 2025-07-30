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
	"time"
)

type MonitorConfig struct {
	MonitorProcess bool   `json:"monitor_process"`
	MonitorPort    bool   `json:"monitor_port"`
	BaseLinePort   string `json:"baseline_port"`
	PortsToMonitor []int  `json:"ports_to_monitor"`
}

type SystemBaseline struct {
	KnownProcess map[string]bool `json:"known_process"`
	KnownPorts   map[string]bool `json:"known_ports"`
}

var (
	config   MonitorConfig
	baseline SystemBaseline
)

// Ham loadConfig su dung de nap cau hinh config phuc vu cho monitor
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

// Ham loadBaseline nap lai baseline (tuc ban ghi truoc) cua giam sat truoc do
func loadBaseline() error {
	if _, err := os.Stat(config.BaseLinePort); os.IsNotExist(err) { // neu baseline.json chua co thi tao struct gom cac map rong
		baseline = SystemBaseline{
			KnownPorts: make(map[string]bool),
		}
		return nil
	}
	file, err := os.ReadFile(config.BaseLinePort)
	if err != nil {
		return fmt.Errorf("read config file failed: %v", err)
	}
	if err := json.Unmarshal(file, &baseline); err != nil {
		return fmt.Errorf("parse config file failed: %v", err)
	}
	return nil
}

// Luu baseline
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

// Cho phep port cho process tuong ung hoac khong
func promptApproval(port int, processName string) bool {
	fmt.Printf("\nDetected port %d is being used by process: %s\n", port, processName)
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("Approval? (y/n): ")
		if !scanner.Scan() {
			fmt.Println("Error reading input.")
			return false
		}
		response := strings.TrimSpace(scanner.Text())
		switch strings.ToLower(response) {
		case "y":
			return true
		case "n":
			return false
		default:
			fmt.Println("Invalid input. Please enter 'y' or 'n'.")
		}
	}
}

// bo qua, khong su dung
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

// Lay ra cac port cung cac process tuong ung cua thiet bi giam sat
func getPortProcessMap() (map[int]string, error) {
	portProcess := make(map[int]string)
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		cmd = exec.Command("netstat", "-ano")
	} else {
		//cmd = exec.Command("lsof", "-i", "-P", "-n", "-F", "pcn")
		// Alternative: Sử dụng lsof không có -F flag
		cmd = exec.Command("lsof", "-i", "-P", "-n")
		// Rồi parse output theo format thông thường
		//cmd = exec.Command("lsof", "-i", "-P", "-n", "+c", "0")
	}

	output, err := cmd.Output() //luu toan bo ket qua in ra man hinh cho cau lenh cmd
	if err != nil {
		return nil, fmt.Errorf("get netstat failed: %v", err)
	}
	//tach thanh tung dong co dau "/" bo qua cac dong co it hon 2 truong
	var port int
	var process string
	//var currentPID, currentCmd string
	lines := strings.Split(string(output), "\n") //tach ra tung dong
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		if runtime.GOOS == "windows" {
			localAddr := fields[1]
			portPart := strings.Split(localAddr, ":") //tach lay port
			if len(portPart) < 2 {
				continue
			}
			portStr := portPart[len(portPart)-1] //lay ptu cuoi
			var err error
			port, err = strconv.Atoi(portStr)
			if err != nil {
				continue
			}

			pid := fields[len(fields)-1] //lay PID tu truong cuoi

			taskCmd := exec.Command("tasklist", "/FI", "PID eq "+pid, "/FO", "CSV", "/NH")

			taskOutput, err := taskCmd.Output()
			if err != nil {
				process = "unknown"
			} else {
				if parts := strings.Split(string(taskOutput), "\""); len(parts) >= 2 {
					process = strings.TrimSpace(parts[1])
					process = strings.TrimSuffix(process, ".exe")
				} else {
					process = "unknown"
				}
			}
			portProcess[port] = process
		} else {
			fields := strings.Fields(line)
			if len(fields) < 9 { // neu it hon 9 truong du lieu, bo qua
				continue
			}
			command := fields[0]
			var netAddr string //tim port bang cach lap qua tu cuoi ve
			for i := len(fields) - 1; i >= 0; i-- {
				if strings.Contains(fields[i], ":") {
					netAddr = fields[i]
					break
				}
			}

			if netAddr == "" {
				continue
			}
			// Tách port từ address
			lastColon := strings.LastIndex(netAddr, ":")
			if lastColon == -1 {
				continue
			}

			portStr := netAddr[lastColon+1:]
			// Làm sạch port string - loại bỏ (LISTEN) nếu có
			portStr = strings.Split(portStr, " ")[0]
			portStr = strings.TrimRight(portStr, "(LISTEN)")

			var err error
			port, err = strconv.Atoi(portStr)
			if err != nil {
				continue
			}

			// Gán process name vào map
			portProcess[port] = command
		}
	}
	//portProcess[port] = process

	return portProcess, nil
}

func checkPorts() {
	if !config.MonitorProcess || len(config.PortsToMonitor) == 0 {
		return
	}
	fmt.Println("Checking ports...")
	newPortsFound := false

	portProcessMap, err := getPortProcessMap() // lay cac port dang chay
	//fmt.Println(portProcessMap)
	if err != nil {
		fmt.Printf("Unable to get port process map: %v", err)
		return
	}
	for _, port := range config.PortsToMonitor { //lap qua port muon quan sat trong config.json
		if process, exists := portProcessMap[port]; exists {
			key := fmt.Sprintf("%d:%s", port, process)
			if !baseline.KnownPorts[key] {
				fmt.Printf("\nALERT: Monitored port %d is being used by process: %s\n", port, process)
				newPortsFound = true
				if promptApproval(port, process) {
					baseline.KnownPorts[key] = true
					if err := saveBaseline(); err != nil {
						fmt.Printf("Error saving baseline: %v\n", err)
					} else {
						fmt.Printf("Port %d with process %s added to baseline\n", port, process)
					}
				} else {
					fmt.Printf("Port %d with process %s is NOT approved\n", port, process)
				}
			} else {
				fmt.Printf("Approved port %d is being used by process: %s\n", port, process)
			}
		}
	}
	if !newPortsFound {
		fmt.Printf("\n No new ports found.\n")
	}
}

func main() {
	//Dam bao nap config.json
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

	fmt.Print("\nPort monitoring program has started\n")

	checkPorts()
	checkInterval := 1 * time.Minute

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			checkPorts()
		}
	}
}
