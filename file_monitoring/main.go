package main

import (
	"bufio"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config cau hinh giam sat folder
type MonitorConfig struct {
	MonitorFolder  []string `json:"monitor_folder"`
	FileExtensions []string `json:"file_extensions"`
	IgnoreFiles    []string `json:"ignore_files"`
	BaseLineFile   string   `json:"baseline_file"`
}

// Trang thai file duoc chap nhan
type FileBaseline struct {
	KnownFiles map[string]bool `json:"known_files"` // path -> exist
}

var (
	config   MonitorConfig
	baseline FileBaseline
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
		baseline = FileBaseline{
			KnownFiles: make(map[string]bool),
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

//func promptApproval(path string) bool {
//	fmt.Printf("\n Detect new files %s \n", path)
//	fmt.Print("Approval? (y/n)")
//	scanner := bufio.NewScanner(os.Stdin)
//	if !scanner.Scan() {
//		return false
//	}
//	response := strings.TrimSpace(scanner.Text())
//	return strings.EqualFold(response, "y")
//}

func promptApproval(path string) bool {
	fmt.Printf("\nDetect new files %s\n", path)
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("Approval? (y/n): ")
		if !scanner.Scan() {
			// Xử lý nếu không đọc được input (EOF hoặc lỗi)
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

func checkFiles() {
	fmt.Printf("\n Checking files...\n")
	newFilesFound := false
	//detectedFiles := make([]string, 0) // luu danh sach file moi phat hien
	for _, folder := range config.MonitorFolder {
		filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				//return err
				fmt.Printf("Warning: Cannot access %s: %v\n", path, err)
				return nil
			}

			if info.IsDir() {
				//Kiem tra neu folder nam trong danh sach thi bo qua
				for _, ignore := range config.IgnoreFiles {
					if strings.Contains(info.Name(), ignore) {
						return filepath.SkipDir
					}
				}
				return nil
			}

			//kiem tra cac file duoc ignore, neu ten file moi trung voi ten duoc ignore thi bo qua (lo hong)
			//for _, ignore := range config.IgnoreFiles {
			//	if strings.Contains(info.Name(), ignore) {
			//		return nil
			//	}
			//}

			if _, exist := baseline.KnownFiles[path]; exist {
				return nil
			}
			// Kiem tra extension
			if len(config.FileExtensions) > 0 {
				ext := filepath.Ext(path)
				valiExt := false
				for _, e := range config.FileExtensions {
					if strings.EqualFold(ext, e) {
						valiExt = true
						break
					}
				}
				if !valiExt {

					fmt.Printf("Warning: File %s not found at file_extensions in %s\n", ext, path)
					//return nil
					if promptApproval(path) {
						baseline.KnownFiles[path] = true
						if err := saveBaseline(); err != nil {
							fmt.Printf("Unable to save baseline file: %v\n", err)
						} else {
							fmt.Printf("Approved file %s not found at file_extensions and saved baseline file: %s\n", ext, path)
						}
					} else {
						err := os.Remove(path)
						if err != nil {
							fmt.Printf("Unable to remove %s: %v\n", path, err)
						} else {
							fmt.Printf("Removed file %s not found at file_extensions: %s\n", ext, path)
						}
					}
				}
			}

			// Kiem tra file moi
			newFilesFound = true
			if promptApproval(path) {
				baseline.KnownFiles[path] = true
				if err := saveBaseline(); err != nil {
					fmt.Printf("Unable to save baseline file: %v\n", err)
				} else {
					fmt.Printf("Approved and saved baseline file: %s\n", path)
				}
			} else {
				if err := os.Remove(path); err != nil {
					fmt.Printf("Unable to remove %s: %v\n", path, err)
				} else {
					fmt.Printf("Removed file: %s\n", path)
				}
			}
			return nil

		})
	}
	if !newFilesFound {
		fmt.Printf("\n No new files found.\n")
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage config_file") //đảm bảo len(os.Args) = 1, điều kiện này đúng, người dùng cần cung cấp file đường dẫn config
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
	checkFiles()

	checkInterval := 1 * time.Minute

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			checkFiles()
		}
	}

}
