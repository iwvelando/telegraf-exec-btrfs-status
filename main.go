package main

import (
	"bufio"
	"code.cloudfoundry.org/bytefmt"
	"flag"
	"fmt"
	"github.com/TobiEiss/go-textfsm/pkg/ast"
	"github.com/TobiEiss/go-textfsm/pkg/process"
	"github.com/TobiEiss/go-textfsm/pkg/reader"
	influx "github.com/influxdata/influxdb-client-go/v2"
	influxWrite "github.com/influxdata/influxdb-client-go/v2/api/write"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var statusInt = map[string]int{
	"running":     0,
	"finished":    1,
	"aborted":     2,
	"interrupted": 3,
}

func main() {

	templateDeviceStats := flag.String(
		"template-device-stats",
		"./btrfs_device_stats_template.txt",
		"path to the TextFSM template for parsing btrfs device stats",
	)

	templateFilesystemUsage := flag.String(
		"template-filesystem-usage",
		"./btrfs_filesystem_usage_template.txt",
		"path to the TextFSM template for parsing btrfs filesystem usage --raw",
	)

	templateScrubStatus := flag.String(
		"template-scrub-status",
		"./btrfs_scrub_status_template.txt",
		"path to the TextFSM template for parsing btrfs scrub status -d",
	)

	fileInputScrub := flag.String(
		"file-input-scrub",
		"",
		"[optional] path to file with scrub status output to be parsed, typically for testing",
	)

	flag.Parse()

	if *fileInputScrub != "" {
		f, err := os.Open(*fileInputScrub)
		if err != nil {
			log.WithFields(log.Fields{
				"op":    "main",
				"error": err,
			}).Error(fmt.Sprintf("failed to open scrub input file %s", *fileInputScrub))
			os.Exit(1)
		}
		defer f.Close()
		scrubData, err := io.ReadAll(f)
		if err != nil {
			log.WithFields(log.Fields{
				"op":    "main",
				"error": err,
			}).Error(fmt.Sprintf("failed to read scrub input file %s", *fileInputScrub))
			os.Exit(1)
		}
		err = ParseBtrfsScrubStatus(*fileInputScrub, scrubData, *templateScrubStatus)
		if err != nil {
			log.WithFields(log.Fields{
				"op":    "main",
				"error": err,
			}).Error(fmt.Sprintf("failed to parse scrub input file %s", *fileInputScrub))
			os.Exit(1)
		}
		os.Exit(0)
	}

	mounts, err := GetBtrfsMounts()
	if err != nil {
		log.WithFields(log.Fields{
			"op":    "main.GetBtrfsMounts",
			"error": err,
		}).Error("failed to enumerate btrfs mounts")
		os.Exit(1)
	}

	for _, mount := range mounts {

		cmd := exec.Command("btrfs", "device", "stats", mount)
		stdout, err := cmd.Output()
		if err != nil {
			log.WithFields(log.Fields{
				"op":    "main",
				"error": err,
			}).Error(fmt.Sprintf("failed to execute btrfs device stats %s", mount))
			os.Exit(2)
		}
		err = ParseBtrfsDeviceStats(mount, stdout, *templateDeviceStats)
		if err != nil {
			log.WithFields(log.Fields{
				"op":    "main.ParseBtrfsDeviceStats",
				"error": err,
			}).Error(fmt.Sprintf("failed to parse btrfs device stats %s", mount))
			os.Exit(3)
		}

		cmd = exec.Command("btrfs", "filesystem", "usage", "--raw", mount)
		stdout, err = cmd.Output()
		if err != nil {
			log.WithFields(log.Fields{
				"op":    "main",
				"error": err,
			}).Error(fmt.Sprintf("failed to execute btrfs filesystem df --raw %s", mount))
			os.Exit(4)
		}
		err = ParseBtrfsFilesystemUsage(mount, stdout, *templateFilesystemUsage)
		if err != nil {
			log.WithFields(log.Fields{
				"op":    "main.ParseBtrfsFilesystemUsage",
				"error": err,
			}).Error(fmt.Sprintf("failed to parse btrfs filesystem df --raw %s", mount))
			os.Exit(5)
		}

		cmd = exec.Command("btrfs", "scrub", "status", "-d", mount)
		stdout, err = cmd.Output()
		if err != nil {
			log.WithFields(log.Fields{
				"op":    "main",
				"error": err,
			}).Error(fmt.Sprintf("failed to execute btrfs scrub status -d %s", mount))
			os.Exit(6)
		}
		err = ParseBtrfsScrubStatus(mount, stdout, *templateScrubStatus)
		if err != nil {
			log.WithFields(log.Fields{
				"op":    "main.ParseBtrfsScrubStatus",
				"error": err,
			}).Error(fmt.Sprintf("failed to parse btrfs scrub status -d %s", mount))
			os.Exit(7)
		}

	}

}

func GetBtrfsMounts() ([]string, error) {
	var mounts []string
	var devices []string

	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return mounts, fmt.Errorf("failed to open /proc/self/mounts; %s", err)
	}
	defer f.Close()

	const (
		deviceIdx = 0
		mountIdx  = 1
		typeIdx   = 2
		options   = 3
	)

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if fields[typeIdx] == "btrfs" {
			device := fields[deviceIdx]
			mount := fields[mountIdx]
			if ElementOf(devices, device) {
				continue
			} else {
				devices = append(devices, device)
				mounts = append(mounts, mount)
			}
		} else {
			continue
		}
	}

	return mounts, nil
}

func ParseBtrfsDeviceStats(mount string, output []byte, templatePath string) error {
	ts := time.Now()
	tmplCh := make(chan string)
	go reader.ReadLineByLine(templatePath, tmplCh)

	srcCh := make(chan string)
	go reader.ReadLineByLineFileAsString(string(output), srcCh)

	ast, err := ast.CreateAST(tmplCh)
	if err != nil {
		return fmt.Errorf("failed to create AST; %s", err)
	}

	record := make(chan []interface{})
	process, err := process.NewProcess(ast, record)
	if err != nil {
		return fmt.Errorf("failed to create record processor; %s", err)
	}
	go process.Do(srcCh)

	for {
		row, ok := <-record

		if !ok {
			break
		}

		deviceName := row[0].(string)

		writeIOErrorsRaw := row[1].(string)
		writeIOErrors, err := strconv.Atoi(writeIOErrorsRaw)
		if err != nil {
			return fmt.Errorf("failed to convert write IO errors to int; %s", err)
		}

		readIOErrorsRaw := row[2].(string)
		readIOErrors, err := strconv.Atoi(readIOErrorsRaw)
		if err != nil {
			return fmt.Errorf("failed to convert read IO errors to int; %s", err)
		}

		flushIOErrorsRaw := row[3].(string)
		flushIOErrors, err := strconv.Atoi(flushIOErrorsRaw)
		if err != nil {
			return fmt.Errorf("failed to convert flush IO errors to int; %s", err)
		}

		corruptionIOErrorsRaw := row[4].(string)
		corruptionIOErrors, err := strconv.Atoi(corruptionIOErrorsRaw)
		if err != nil {
			return fmt.Errorf("failed to convert corruption IO errors to int; %s", err)
		}

		generationIOErrorsRaw := row[5].(string)
		generationIOErrors, err := strconv.Atoi(generationIOErrorsRaw)
		if err != nil {
			return fmt.Errorf("failed to convert generation IO errors to int; %s", err)
		}

		data := influx.NewPoint(
			"btrfs_device_errors",
			map[string]string{
				"mount":  mount,
				"device": deviceName,
			},
			map[string]interface{}{
				"write_io_errors":      writeIOErrors,
				"read_io_errors":       readIOErrors,
				"flush_io_errors":      flushIOErrors,
				"corruption_io_errors": corruptionIOErrors,
				"generation_io_errors": generationIOErrors,
			},
			ts,
		)

		fmt.Println(influxWrite.PointToLineProtocol(data, time.Nanosecond))
	}
	return nil
}

func ParseBtrfsFilesystemUsage(mount string, output []byte, templatePath string) error {
	ts := time.Now()
	tmplCh := make(chan string)
	go reader.ReadLineByLine(templatePath, tmplCh)

	srcCh := make(chan string)
	go reader.ReadLineByLineFileAsString(string(output), srcCh)

	ast, err := ast.CreateAST(tmplCh)
	if err != nil {
		return fmt.Errorf("failed to create AST; %s", err)
	}

	record := make(chan []interface{})
	process, err := process.NewProcess(ast, record)
	if err != nil {
		return fmt.Errorf("failed to create record processor; %s", err)
	}
	go process.Do(srcCh)

	for {
		row, ok := <-record

		if !ok {
			break
		}

		if row[0].(string) != "" {
			// Overall section
			filesystemSize, err := strconv.Atoi(row[0].(string))
			if err != nil {
				return fmt.Errorf("failed to convert filesystem size to int; %s", err)
			}

			filesystemAllocated, err := strconv.Atoi(row[1].(string))
			if err != nil {
				return fmt.Errorf("failed to convert filesystem allocated to int; %s", err)
			}

			filesystemUnallocated, err := strconv.Atoi(row[2].(string))
			if err != nil {
				return fmt.Errorf("failed to convert filesystem unallocated to int; %s", err)
			}

			filesystemMissing, err := strconv.Atoi(row[3].(string))
			if err != nil {
				return fmt.Errorf("failed to convert filesystem missing to int; %s", err)
			}

			filesystemUsed, err := strconv.Atoi(row[4].(string))
			if err != nil {
				return fmt.Errorf("failed to convert filesystem used to int; %s", err)
			}

			filesystemFreeEstimated, err := strconv.Atoi(row[5].(string))
			if err != nil {
				return fmt.Errorf("failed to convert filesystem free estimated to int; %s", err)
			}

			filesystemFreeEstimatedMin, err := strconv.Atoi(row[6].(string))
			if err != nil {
				return fmt.Errorf("failed to convert filesystem free estimated min to int; %s", err)
			}

			filesystemDataRatio, err := strconv.ParseFloat(row[7].(string), 32)
			if err != nil {
				return fmt.Errorf("failed to convert filesystem data ratio to float; %s", err)
			}

			filesystemMetadataRatio, err := strconv.ParseFloat(row[8].(string), 32)
			if err != nil {
				return fmt.Errorf("failed to convert filesystem metadata ratio to float; %s", err)
			}

			filesystemGlobalReserve, err := strconv.Atoi(row[9].(string))
			if err != nil {
				return fmt.Errorf("failed to convert filesystem global rserve to int; %s", err)
			}

			filesystemGlobalReserveUsed, err := strconv.Atoi(row[10].(string))
			if err != nil {
				return fmt.Errorf("failed to convert filesystem global rserve used to int; %s", err)
			}

			data := influx.NewPoint(
				"btrfs_filesystem",
				map[string]string{
					"mount":  mount,
					"aspect": "Overall",
				},
				map[string]interface{}{
					"filesystem_size":                filesystemSize,
					"filesystem_allocated":           filesystemAllocated,
					"filesystem_unallocated":         filesystemUnallocated,
					"filesystem_missing":             filesystemMissing,
					"filesystem_used":                filesystemUsed,
					"filesystem_free_estimated":      filesystemFreeEstimated,
					"filesystem_free_estimated_min":  filesystemFreeEstimatedMin,
					"filesystem_data_ratio":          filesystemDataRatio,
					"filesystem_metadata_ratio":      filesystemMetadataRatio,
					"filesystem_global_reserve":      filesystemGlobalReserve,
					"filesystem_global_reserve_used": filesystemGlobalReserveUsed,
				},
				ts,
			)

			fmt.Println(influxWrite.PointToLineProtocol(data, time.Nanosecond))
		} else if row[12].(string) != "" {
			// Aspect summary section
			aspect := row[11].(string)

			filesystemType := row[12].(string)

			filesystemSize, err := strconv.Atoi(row[13].(string))
			if err != nil {
				return fmt.Errorf("failed to convert aspect filesystem size to int; %s", err)
			}

			filesystemUsed, err := strconv.Atoi(row[14].(string))
			if err != nil {
				return fmt.Errorf("failed to convert aspect filesystem used to int; %s", err)
			}

			filesystemUsedPercent, err := strconv.ParseFloat(row[15].(string), 32)
			if err != nil {
				return fmt.Errorf("failed to convert aspect filesystem used percent to float; %s", err)
			}

			data := influx.NewPoint(
				"btrfs_filesystem",
				map[string]string{
					"mount":  mount,
					"aspect": aspect,
					"type":   filesystemType,
				},
				map[string]interface{}{
					"filesystem_size":         filesystemSize,
					"filesystem_used":         filesystemUsed,
					"filesystem_used_percent": filesystemUsedPercent,
				},
				ts,
			)

			fmt.Println(influxWrite.PointToLineProtocol(data, time.Nanosecond))
		} else if row[16].(string) != "" {
			// Aspect device section
			aspect := row[11].(string)

			deviceName := row[16].(string)

			deviceSize, err := strconv.Atoi(row[17].(string))
			if err != nil {
				return fmt.Errorf("failed to convert aspect device size to int; %s", err)
			}

			data := influx.NewPoint(
				"btrfs_filesystem",
				map[string]string{
					"mount":  mount,
					"aspect": aspect,
					"device": deviceName,
				},
				map[string]interface{}{
					"device_size": deviceSize,
				},
				ts,
			)

			fmt.Println(influxWrite.PointToLineProtocol(data, time.Nanosecond))
		}

	}

	return nil
}

func ParseBtrfsScrubStatus(mount string, output []byte, templatePath string) error {
	ts := time.Now()
	tmplCh := make(chan string)
	go reader.ReadLineByLine(templatePath, tmplCh)

	srcCh := make(chan string)
	go reader.ReadLineByLineFileAsString(string(output), srcCh)

	ast, err := ast.CreateAST(tmplCh)
	if err != nil {
		return fmt.Errorf("failed to create AST; %s", err)
	}

	record := make(chan []interface{})
	process, err := process.NewProcess(ast, record)
	if err != nil {
		return fmt.Errorf("failed to create record processor; %s", err)
	}
	go process.Do(srcCh)

	for {
		row, ok := <-record

		if !ok {
			break
		}

		deviceName, ok := row[0].(string)
		if !ok {
			break
		}

		deviceId := row[1].(string)

		startTimeRaw := row[2].(string)
		if startTimeRaw == "" {
			break
		}
		timeLayout := "Mon Jan 2 15:04:05 2006"
		t, err := time.ParseInLocation(timeLayout, startTimeRaw, time.Local)
		if err != nil {
			return fmt.Errorf("failed to parse scrub start time; %s", err)
		}
		startTime := t.Unix()

		var status int
		statusRaw := row[3].(string)
		if status, ok = statusInt[statusRaw]; !ok {
			status = 99
		}

		durationRaw := row[4].(string)
		durationSplit := strings.Split(durationRaw, ":")
		durationPreformatted := fmt.Sprintf("%sh%sm%ss",
			durationSplit[0], durationSplit[1], durationSplit[2])
		d, err := time.ParseDuration(durationPreformatted)
		if err != nil {
			return fmt.Errorf("failed to parse scrub destination; %s", err)
		}
		duration := int(d.Seconds())

		total, err := bytefmt.ToBytes(row[5].(string))
		if err != nil {
			return fmt.Errorf(fmt.Sprintf("failed to parse %s; %s", row[5].(string), err))
		}

		rate, err := bytefmt.ToBytes(strings.TrimSuffix(row[6].(string), "/s"))
		if err != nil {
			return fmt.Errorf(fmt.Sprintf("failed to parse %s; %s", row[6].(string), err))
		}

		var readErrors, superErrors, verifyErrors, checksumErrors,
			correctedErrors, uncorrectableErrors, unverifiedErrors int
		if row[11].(string) == "" {
			readErrors = 0
			superErrors = 0
			verifyErrors = 0
			checksumErrors = 0
			correctedErrors = 0
			uncorrectableErrors = 0
			unverifiedErrors = 0
		} else {
			if row[7].(string) != "" {
				readErrors, err = strconv.Atoi(row[7].(string))
				if err != nil {
					return fmt.Errorf("failed to convert read errors to int; %s", err)
				}
			} else {
				readErrors = 0
			}

			if row[8].(string) != "" {
				superErrors, err = strconv.Atoi(row[8].(string))
				if err != nil {
					return fmt.Errorf("failed to convert super errors to int; %s", err)
				}
			} else {
				superErrors = 0
			}

			if row[9].(string) != "" {
				verifyErrors, err = strconv.Atoi(row[9].(string))
				if err != nil {
					return fmt.Errorf("failed to convert verify errors to int; %s", err)
				}
			} else {
				verifyErrors = 0
			}

			if row[10].(string) != "" {
				checksumErrors, err = strconv.Atoi(row[10].(string))
				if err != nil {
					return fmt.Errorf("failed to convert checksum errors to int; %s", err)
				}
			} else {
				checksumErrors = 0
			}

			correctedErrors, err = strconv.Atoi(row[11].(string))
			if err != nil {
				return fmt.Errorf("failed to convert corrected errors to int; %s", err)
			}

			uncorrectableErrors, err = strconv.Atoi(row[12].(string))
			if err != nil {
				return fmt.Errorf("failed to convert uncorrectable errors to int; %s", err)
			}

			unverifiedErrors, err = strconv.Atoi(row[13].(string))
			if err != nil {
				return fmt.Errorf("failed to convert unverified errors to int; %s", err)
			}
		}

		data := influx.NewPoint(
			"btrfs_scrub",
			map[string]string{
				"mount":     mount,
				"device":    deviceName,
				"device_id": deviceId,
			},
			map[string]interface{}{
				"start":                startTime,
				"status":               status,
				"duration":             duration,
				"total":                total,
				"rate":                 rate,
				"read_errors":          readErrors,
				"super_errors":         superErrors,
				"verify_errors":        verifyErrors,
				"checksum_errors":      checksumErrors,
				"corrected_errors":     correctedErrors,
				"uncorrectable_errors": uncorrectableErrors,
				"unverified_errors":    unverifiedErrors,
			},
			ts,
		)

		fmt.Println(influxWrite.PointToLineProtocol(data, time.Nanosecond))
	}
	return nil
}

func ElementOf(slice []string, val string) bool {

	for _, item := range slice {
		if item == val {
			return true
		}
	}

	return false
}
