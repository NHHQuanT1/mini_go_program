package main

import (
	"bufio"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Config cau hinh giam sat folder
type MonitorConfig struct {
	MonitorFolder  []string `json:"monitor_folder"`
	FileExtensions []string `json:"file_extensions"`
	IgnoreFiles    []string `json:"ignore_files"`
	BaseLineFile   string   `json:"baseline_file"`
	MonitorProcess bool     `json:"monitor_process"`
}

// Trang thai file duoc chap nhan
type SystemBaseline struct {
	KnownProcess map[string]bool `json:"known_process"`
	KnownFiles   map[string]bool `json:"known_files"`
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
	if _, err := os.Stat(config.BaseLineFile); os.IsNotExist(err) {
		baseline = SystemBaseline{
			KnownProcess: make(map[string]bool),
		}
		return nil
	}
	file, err := os.ReadFile(config.BaseLineFile)
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
	if err := os.WriteFile(config.BaseLineFile, data, 0644); err != nil {
		return fmt.Errorf("unable to write baseline file: %v", err)
	}
	return nil
}

func getFileHash(filePath string) (string, error) {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("unable to read file: %v", err)
	}
	return fmt.Sprintf("%x", md5.Sum(file)), nil
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
					processes = append(processes, strings.ToLower(processName))
				}
			}
		} else {
			processName := strings.ToLower(line)
			processes = append(processes, processName)
		}
	}
	return processes, nil

}

func checkProcesses() {
	if !config.MonitorProcess {
		return
	}
	fmt.Println("Checking processes...")
	newProcessesFound := false

	currentProcesses, err := getRunningProcesses()
	if err != nil {
		fmt.Printf("Unable to get running processes: %v", err)
		return
	}
	for _, name := range currentProcesses {
		if _, exits := baseline.KnownProcess[name]; !exits {
			newProcessesFound = true
			if promptApproval(name) {
				baseline.KnownProcess[name] = true
				if err := saveBaseline(); err != nil {
					fmt.Printf("Unable to save baseline file: %v", err)
				} else {
					fmt.Printf("Saved baseline file: %s", name)
				}
			} else {
				fmt.Printf("Unapproved process detected: %s \n", name)
			}
		}
	}
	if !newProcessesFound {
		fmt.Println("No new processes detected")
	}
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

	fmt.Print("\n File monitoring program has started \n")
	fmt.Printf("\n Monitoring %d folder \n", len(config.MonitorFolder))
	//checkFiles()
	checkProcesses()
	//checkInterval := 1 * time.Minute
	//
	//ticker := time.NewTicker(checkInterval)
	//defer ticker.Stop()
	//
	//for {
	//	select {
	//	case <-ticker.C:
	//		//checkFiles()
	//	}
	//}

}
