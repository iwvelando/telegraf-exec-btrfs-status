package main

import (
	"bufio"
	"code.cloudfoundry.org/bytefmt"
	"flag"
	"fmt"
	"github.com/TobiEiss/go-textfsm/pkg/ast"
	"github.com/TobiEiss/go-textfsm/pkg/process"
	"github.com/TobiEiss/go-textfsm/pkg/reader"
	_ "github.com/influxdata/influxdb1-client"
	influxdb "github.com/influxdata/influxdb1-client"
	"go.uber.org/zap"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func main() {

	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Println("{\"op\": \"main\", \"level\": \"fatal\", \"msg\": \"failed to initiate logger\"}")
		os.Exit(1)
	}
	defer logger.Sync()

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
	flag.Parse()

	mounts, err := GetBtrfsMounts(logger)
	if err != nil {
		logger.Error("failed to enumerate btrfs mounts",
			zap.String("op", "main"),
			zap.Error(err),
		)
		os.Exit(1)
	}

	for _, mount := range mounts {

		cmd := exec.Command("btrfs", "device", "stats", mount)
		stdout, err := cmd.Output()
		if err != nil {
			logger.Error(fmt.Sprintf("failed to execute btrfs device stats %s", mount),
				zap.String("op", "main"),
				zap.Error(err),
			)
			os.Exit(2)
		}
		err = ParseBtrfsDeviceStats(logger, mount, stdout, *templateDeviceStats)
		if err != nil {
			os.Exit(3)
		}

		cmd = exec.Command("btrfs", "filesystem", "usage", "--raw", mount)
		stdout, err = cmd.Output()
		if err != nil {
			logger.Error(fmt.Sprintf("failed to execute btrfs filesystem df --raw %s", mount),
				zap.String("op", "main"),
				zap.Error(err),
			)
			os.Exit(4)
		}
		err = ParseBtrfsFilesystemUsage(logger, mount, stdout, *templateFilesystemUsage)
		if err != nil {
			os.Exit(5)
		}

		cmd = exec.Command("btrfs", "scrub", "status", "-d", mount)
		stdout, err = cmd.Output()
		if err != nil {
			logger.Error(fmt.Sprintf("failed to execute btrfs scrub status -d %s", mount),
				zap.String("op", "main"),
				zap.Error(err),
			)
			os.Exit(6)
		}
		err = ParseBtrfsScrubStatus(logger, mount, stdout, *templateScrubStatus)
		if err != nil {
			os.Exit(7)
		}

	}

}

func GetBtrfsMounts(logger *zap.Logger) ([]string, error) {
	var mounts []string
	var devices []string

	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		logger.Error("failed to open /proc/self/mounts",
			zap.String("op", "GetBtrfsMounts"),
			zap.Error(err),
		)
		return mounts, err
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

func ParseBtrfsDeviceStats(logger *zap.Logger, mount string, output []byte, templatePath string) error {
	ts := time.Now()
	tmplCh := make(chan string)
	go reader.ReadLineByLine(templatePath, tmplCh)

	srcCh := make(chan string)
	go reader.ReadLineByLineFileAsString(string(output), srcCh)

	ast, err := ast.CreateAST(tmplCh)
	if err != nil {
		logger.Error("failed to create AST",
			zap.String("op", "ParseBtrfsDeviceStats"),
			zap.Error(err),
		)
		return err
	}

	record := make(chan []interface{})
	process, err := process.NewProcess(ast, record)
	if err != nil {
		logger.Error("failed to create record processor",
			zap.String("op", "ParseBtrfsDeviceStats"),
			zap.Error(err),
		)
		return err
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
			logger.Error("failed to convert write IO errors to int",
				zap.String("op", "ParseBtrfsDeviceStats"),
				zap.Error(err),
			)
			continue
		}

		readIOErrorsRaw := row[2].(string)
		readIOErrors, err := strconv.Atoi(readIOErrorsRaw)
		if err != nil {
			logger.Error("failed to convert read IO errors to int",
				zap.String("op", "ParseBtrfsDeviceStats"),
				zap.Error(err),
			)
			continue
		}

		flushIOErrorsRaw := row[3].(string)
		flushIOErrors, err := strconv.Atoi(flushIOErrorsRaw)
		if err != nil {
			logger.Error("failed to convert flush IO errors to int",
				zap.String("op", "ParseBtrfsDeviceStats"),
				zap.Error(err),
			)
			continue
		}

		corruptionIOErrorsRaw := row[4].(string)
		corruptionIOErrors, err := strconv.Atoi(corruptionIOErrorsRaw)
		if err != nil {
			logger.Error("failed to convert corruption IO errors to int",
				zap.String("op", "ParseBtrfsDeviceStats"),
				zap.Error(err),
			)
			continue
		}

		generationIOErrorsRaw := row[5].(string)
		generationIOErrors, err := strconv.Atoi(generationIOErrorsRaw)
		if err != nil {
			logger.Error("failed to convert generation IO errors to int",
				zap.String("op", "ParseBtrfsDeviceStats"),
				zap.Error(err),
			)
			continue
		}

		data := influxdb.Point{
			Measurement: "btrfs_device_errors",
			Tags: map[string]string{
				"mount":  mount,
				"device": deviceName,
			},
			Fields: map[string]interface{}{
				"write_io_errors":      writeIOErrors,
				"read_io_errors":       readIOErrors,
				"flush_io_errors":      flushIOErrors,
				"corruption_io_errors": corruptionIOErrors,
				"generation_io_errors": generationIOErrors,
			},
			Time:      ts,
			Precision: "ns",
		}

		fmt.Println(data.MarshalString())
	}
	return nil
}

func ParseBtrfsFilesystemUsage(logger *zap.Logger, mount string, output []byte, templatePath string) error {
	ts := time.Now()
	tmplCh := make(chan string)
	go reader.ReadLineByLine(templatePath, tmplCh)

	srcCh := make(chan string)
	go reader.ReadLineByLineFileAsString(string(output), srcCh)

	ast, err := ast.CreateAST(tmplCh)
	if err != nil {
		logger.Error("failed to create AST",
			zap.String("op", "ParseBtrfsFilesystemUsage"),
			zap.Error(err),
		)
		return err
	}

	record := make(chan []interface{})
	process, err := process.NewProcess(ast, record)
	if err != nil {
		logger.Error("failed to create record processor",
			zap.String("op", "ParseBtrfsFilesystemUsage"),
			zap.Error(err),
		)
		return err
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
				logger.Error("failed to convert filesystem size to int",
					zap.String("op", "ParseBtrfsFilesystemUsage"),
					zap.Error(err),
				)
				continue
			}

			filesystemAllocated, err := strconv.Atoi(row[1].(string))
			if err != nil {
				logger.Error("failed to convert filesystem allocated to int",
					zap.String("op", "ParseBtrfsFilesystemUsage"),
					zap.Error(err),
				)
				continue
			}

			filesystemUnallocated, err := strconv.Atoi(row[2].(string))
			if err != nil {
				logger.Error("failed to convert filesystem unallocated to int",
					zap.String("op", "ParseBtrfsFilesystemUsage"),
					zap.Error(err),
				)
				continue
			}

			filesystemMissing, err := strconv.Atoi(row[3].(string))
			if err != nil {
				logger.Error("failed to convert filesystem missing to int",
					zap.String("op", "ParseBtrfsFilesystemUsage"),
					zap.Error(err),
				)
				continue
			}

			filesystemUsed, err := strconv.Atoi(row[4].(string))
			if err != nil {
				logger.Error("failed to convert filesystem used to int",
					zap.String("op", "ParseBtrfsFilesystemUsage"),
					zap.Error(err),
				)
				continue
			}

			filesystemFreeEstimated, err := strconv.Atoi(row[5].(string))
			if err != nil {
				logger.Error("failed to convert filesystem free estimated to int",
					zap.String("op", "ParseBtrfsFilesystemUsage"),
					zap.Error(err),
				)
				continue
			}

			filesystemFreeEstimatedMin, err := strconv.Atoi(row[6].(string))
			if err != nil {
				logger.Error("failed to convert filesystem free estimated min to int",
					zap.String("op", "ParseBtrfsFilesystemUsage"),
					zap.Error(err),
				)
				continue
			}

			filesystemDataRatio, err := strconv.ParseFloat(row[7].(string), 32)
			if err != nil {
				logger.Error("failed to convert filesystem data ratio to float",
					zap.String("op", "ParseBtrfsFilesystemUsage"),
					zap.Error(err),
				)
				continue
			}

			filesystemMetadataRatio, err := strconv.ParseFloat(row[8].(string), 32)
			if err != nil {
				logger.Error("failed to convert filesystem metadata ratio to float",
					zap.String("op", "ParseBtrfsFilesystemUsage"),
					zap.Error(err),
				)
				continue
			}

			filesystemGlobalReserve, err := strconv.Atoi(row[9].(string))
			if err != nil {
				logger.Error("failed to convert filesystem global rserve to int",
					zap.String("op", "ParseBtrfsFilesystemUsage"),
					zap.Error(err),
				)
				continue
			}

			filesystemGlobalReserveUsed, err := strconv.Atoi(row[10].(string))
			if err != nil {
				logger.Error("failed to convert filesystem global rserve used to int",
					zap.String("op", "ParseBtrfsFilesystemUsage"),
					zap.Error(err),
				)
				continue
			}

			data := influxdb.Point{
				Measurement: "btrfs_filesystem",
				Tags: map[string]string{
					"mount":  mount,
					"aspect": "Overall",
				},
				Fields: map[string]interface{}{
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
				Time:      ts,
				Precision: "ns",
			}

			fmt.Println(data.MarshalString())
		} else if row[12].(string) != "" {
			// Aspect summary section
			aspect := row[11].(string)

			filesystemType := row[12].(string)

			filesystemSize, err := strconv.Atoi(row[13].(string))
			if err != nil {
				logger.Error("failed to convert aspect filesystem size to int",
					zap.String("op", "ParseBtrfsFilesystemUsage"),
					zap.Error(err),
				)
				continue
			}

			filesystemUsed, err := strconv.Atoi(row[14].(string))
			if err != nil {
				logger.Error("failed to convert aspect filesystem used to int",
					zap.String("op", "ParseBtrfsFilesystemUsage"),
					zap.Error(err),
				)
				continue
			}

			filesystemUsedPercent, err := strconv.ParseFloat(row[15].(string), 32)
			if err != nil {
				logger.Error("failed to convert aspect filesystem used percent to float",
					zap.String("op", "ParseBtrfsFilesystemUsage"),
					zap.Error(err),
				)
				continue
			}

			data := influxdb.Point{
				Measurement: "btrfs_filesystem",
				Tags: map[string]string{
					"mount":  mount,
					"aspect": aspect,
					"type":   filesystemType,
				},
				Fields: map[string]interface{}{
					"filesystem_size":         filesystemSize,
					"filesystem_used":         filesystemUsed,
					"filesystem_used_percent": filesystemUsedPercent,
				},
				Time:      ts,
				Precision: "ns",
			}

			fmt.Println(data.MarshalString())
		} else if row[16].(string) != "" {
			// Aspect device section
			aspect := row[11].(string)

			deviceName := row[16].(string)

			deviceSize, err := strconv.Atoi(row[17].(string))
			if err != nil {
				logger.Error("failed to convert aspect device size to int",
					zap.String("op", "ParseBtrfsFilesystemUsage"),
					zap.Error(err),
				)
				continue
			}

			data := influxdb.Point{
				Measurement: "btrfs_filesystem",
				Tags: map[string]string{
					"mount":  mount,
					"aspect": aspect,
					"device": deviceName,
				},
				Fields: map[string]interface{}{
					"device_size": deviceSize,
				},
				Time:      ts,
				Precision: "ns",
			}

			fmt.Println(data.MarshalString())
		}

	}

	return nil
}

func ParseBtrfsScrubStatus(logger *zap.Logger, mount string, output []byte, templatePath string) error {
	ts := time.Now()
	tmplCh := make(chan string)
	go reader.ReadLineByLine(templatePath, tmplCh)

	srcCh := make(chan string)
	go reader.ReadLineByLineFileAsString(string(output), srcCh)

	ast, err := ast.CreateAST(tmplCh)
	if err != nil {
		logger.Error("failed to create AST",
			zap.String("op", "ParseBtrfsScrubStatus"),
			zap.Error(err),
		)
		return err
	}

	record := make(chan []interface{})
	process, err := process.NewProcess(ast, record)
	if err != nil {
		logger.Error("failed to create record processor",
			zap.String("op", "ParseBtrfsScrubStatus"),
			zap.Error(err),
		)
		return err
	}
	go process.Do(srcCh)

	for {
		row, ok := <-record

		if !ok {
			break
		}

		deviceName := row[0].(string)

		deviceId := row[1].(string)

		startTimeRaw := row[2].(string)
		timeLayout := "Mon Jan 2 15:04:05 2006"
		t, err := time.ParseInLocation(timeLayout, startTimeRaw, time.Local)
		if err != nil {
			logger.Error("failed to parse scrub start time",
				zap.String("op", "ParseBtrfsScrubStatus"),
				zap.Error(err),
			)
			continue
		}
		startTime := t.Unix()

		status := row[3].(string)

		durationRaw := row[4].(string)
		durationSplit := strings.Split(durationRaw, ":")
		durationPreformatted := fmt.Sprintf("%sh%sm%ss",
			durationSplit[0], durationSplit[1], durationSplit[2])
		d, err := time.ParseDuration(durationPreformatted)
		if err != nil {
			logger.Error("failed to parse scrub duration",
				zap.String("op", "ParseBtrfsScrubStatus"),
				zap.Error(err),
			)
			continue
		}
		duration := int(d.Seconds())

		total, err := bytefmt.ToBytes(row[5].(string))
		if err != nil {
			logger.Error(fmt.Sprintf("failed to parse %s", row[5].(string)),
				zap.String("op", "ParseBtrfsScrubStatus"),
				zap.Error(err),
			)
			continue
		}

		rate, err := bytefmt.ToBytes(strings.TrimSuffix(row[6].(string), "/s"))
		if err != nil {
			logger.Error(fmt.Sprintf("failed to parse %s", row[6].(string)),
				zap.String("op", "ParseBtrfsScrubStatus"),
				zap.Error(err),
			)
			continue
		}

		var checksumErrors, correctedErrors,
			uncorrectableErrors, unverifiedErrors int
		if row[7].(string) == "" {
			checksumErrors = 0
			correctedErrors = 0
			uncorrectableErrors = 0
			unverifiedErrors = 0
		} else {
			checksumErrors, err = strconv.Atoi(row[7].(string))
			if err != nil {
				logger.Error("failed to convert checksum errors to int",
					zap.String("op", "ParseBtrfsScrubStatus"),
					zap.Error(err),
				)
				continue
			}

			correctedErrors, err = strconv.Atoi(row[8].(string))
			if err != nil {
				logger.Error("failed to convert corrected errors to int",
					zap.String("op", "ParseBtrfsScrubStatus"),
					zap.Error(err),
				)
				continue
			}

			uncorrectableErrors, err = strconv.Atoi(row[9].(string))
			if err != nil {
				logger.Error("failed to convert uncorrectable errors to int",
					zap.String("op", "ParseBtrfsScrubStatus"),
					zap.Error(err),
				)
				continue
			}

			unverifiedErrors, err = strconv.Atoi(row[10].(string))
			if err != nil {
				logger.Error("failed to convert unverified errors to int",
					zap.String("op", "ParseBtrfsScrubStatus"),
					zap.Error(err),
				)
				continue
			}
		}

		data := influxdb.Point{
			Measurement: "btrfs_scrub",
			Tags: map[string]string{
				"mount":     mount,
				"device":    deviceName,
				"device_id": deviceId,
			},
			Fields: map[string]interface{}{
				"start":                startTime,
				"status":               status,
				"duration":             duration,
				"total":                total,
				"rate":                 rate,
				"checksum_errors":      checksumErrors,
				"corrected_errors":     correctedErrors,
				"uncorrectable_errors": uncorrectableErrors,
				"unverified_errors":    unverifiedErrors,
			},
			Time:      ts,
			Precision: "ns",
		}

		fmt.Println(data.MarshalString())
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
