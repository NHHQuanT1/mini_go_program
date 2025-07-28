package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Config cau hinh giam sat process
type MonitorConfig struct {
	MonitorFolder    []string `json:"monitor_folder"`
	FileExtensions   []string `json:"file_extensions"`
	IgnoreFiles      []string `json:"ignore_files"`
	BaseLineProcess  string   `json:"baseline_process"`
	MonitorProcess   bool     `json:"monitor_process"`
	ProcessToMonitor []string `json:"process_to_monitor"`
}

// Trang thai process duoc chap nhan
type SystemBaseline struct {
	KnownProcess map[string]bool `json:"known_process"`
}

var (
	config   MonitorConfig
	baseline SystemBaseline
)

func loadConfig(configPath string) error {
	file, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("unable to open config file: %v", err)
	}
	if err := json.Unmarshal(file, &config); err != nil { // file day chinh la data [] bytes doc tu configPath
		return fmt.Errorf("unable to parse config file: %v", err)
	}
	return nil
}

func loadBaseline() error {
	if _, err := os.Stat(config.BaseLineProcess); os.IsNotExist(err) {
		baseline = SystemBaseline{
			KnownProcess: make(map[string]bool),
		}
		return nil
	}
	file, err := os.ReadFile(config.BaseLineProcess)
	if err != nil {
		return fmt.Errorf("unable to read baseline file: %v", err)
	}
	if err := json.Unmarshal(file, &baseline); err != nil {
		return fmt.Errorf("unable to parse baseline file: %v", err)
	}
	return nil
}

func saveBaseline() error {
	data, err := json.MarshalIndent(baseline, "", " ")
	if err != nil {
		return fmt.Errorf("unable to marshal baseline file: %v", err)
	}
	if err := os.WriteFile(config.BaseLineProcess, data, 0644); err != nil {
		return fmt.Errorf("unable to write baseline file: %v", err)
	}
	return nil
}

func promptApproval(processName string) bool {
	fmt.Printf("\n Detect new process %s\n", processName)
	fmt.Print("Approval? (y/n)")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}
	response := strings.TrimSpace(scanner.Text())
	return strings.EqualFold(response, "y")
}

func normalizeProcessName(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(strings.TrimSuffix(name, ".exe"))
	}
	return strings.ToLower(name)
}

func getRunningProcesses() ([]string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("tasklist", "/fo", "csv", "/nh")
	} else {
		cmd = exec.Command("ps", "-e", "-o", "comm=")
	}
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("unable to get running processes: %v", err)
	}

	var processes []string
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if runtime.GOOS == "windows" {
			//tach ten process tu output dang csv
			if parts := strings.Split(line, "\""); len(parts) >= 2 {
				processName := strings.TrimSpace(parts[1])
				if processName != "" {
					processName = strings.ToLower(processName)
					processName = strings.TrimSuffix(processName, ".exe")
					processes = append(processes, strings.ToLower(processName))
				}
			}
		} else {
			processName := strings.ToLower(line)
			// xu ly lay ten process
			if strings.Contains(processName, "/") {
				parts := strings.Split(processName, "/")
				processName = parts[len(parts)-1]
			}
			processes = append(processes, processName)
		}
	}
	return processes, nil

}

func checkProcesses() {
	if !config.MonitorProcess || len(config.ProcessToMonitor) == 0 {
		return
	}
	fmt.Println("Checking processes...")
	//newProcessesFound := false

	currentProcesses, err := getRunningProcesses()
	if err != nil {
		fmt.Printf("Unable to get running processes: %v", err)
		return
	}

	// Tạo map các process đang chạy để kiểm tra nhanh
	runningProcesses := make(map[string]bool)
	for _, proc := range currentProcesses {
		runningProcesses[normalizeProcessName(proc)] = true
	}
	// Kiểm tra các process cần theo dõi
	for _, monitoredProc := range config.ProcessToMonitor {
		normalizedMonitoredProc := normalizeProcessName(monitoredProc)

		if runningProcesses[normalizedMonitoredProc] {
			if !baseline.KnownProcess[normalizedMonitoredProc] {
				//newProcessesFound = true
				fmt.Printf("\nALERT: Monitored process is running: %s\n", monitoredProc)
				fmt.Print("Do you want to allow this process? (y/n): ")

				scanner := bufio.NewScanner(os.Stdin)
				if scanner.Scan() {
					response := strings.TrimSpace(scanner.Text())
					if strings.EqualFold(response, "y") {
						baseline.KnownProcess[normalizedMonitoredProc] = true
						if err := saveBaseline(); err != nil {
							fmt.Printf("Error saving baseline: %v\n", err)
						} else {
							fmt.Printf("Process %s added to baseline\n", monitoredProc)
						}
					} else {
						fmt.Printf("Process %s is NOT approved\n", monitoredProc)
					}
				}
			} else {
				fmt.Printf("Approved process is running: %s\n", monitoredProc)
			}
		}
	}
	//for _, name := range currentProcesses {
	//	if _, exits := baseline.KnownProcess[name]; !exits {
	//		newProcessesFound = true
	//		if promptApproval(name) {
	//			baseline.KnownProcess[name] = true
	//			if err := saveBaseline(); err != nil {
	//				fmt.Printf("Unable to save baseline file: %v", err)
	//			} else {
	//				fmt.Printf("Saved baseline file: %s", name)
	//			}
	//		} else {
	//			fmt.Printf("Unapproved process detected: %s \n", name)
	//		}
	//	}
	//}
	//if !newProcessesFound {
	//	fmt.Println("No new processes detected")
	//}

}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: ./program <config_file>") //đảm bảo len(os.Args) = 1, điều kiện này đúng, người dùng cần cung cấp file đường dẫn config
		os.Exit(1)
	}

	if err := loadConfig(os.Args[1]); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if err := loadBaseline(); err != nil { //load lại trạng thái được lưu trước đó
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Print("\n Processes monitoring program has started \n")
	//fmt.Printf("\n Monitoring %d folder \n", len(config.MonitorFolder))
	//checkFiles()
	checkProcesses()
	checkInterval := 1 * time.Minute

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			checkProcesses()
		}
	}

}
