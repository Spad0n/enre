package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type ProcessResult struct {
	Shell      string
	Stdout     []byte
	Stderr     []byte
	ReturnCode int
}

func handleError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}
}

func capture(snapshots []ProcessResult, index int, command string, wg *sync.WaitGroup, mu *sync.Mutex, semaphore chan struct{}) {
	if wg != nil {
		defer wg.Done()
	}
	if semaphore != nil {
		semaphore <- struct{}{}
	}
	args := strings.Split(command, " ")
	fmt.Printf("CAPTURING: %s\n", args[0])
	cmd := exec.Command(args[0], args[1:]...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
		}
	}

	if mu != nil {
		mu.Lock()
	}
	snapshots[index] = ProcessResult {
		Shell:      args[0],
		Stdout:     stdout.Bytes(),
		Stderr:     stderr.Bytes(),
		ReturnCode: exitCode,
	}
	if mu != nil {
		mu.Unlock()
	}

	if semaphore != nil {
		<-semaphore
	}
}

func loadList(filePath string) []string {
	file, err := os.Open(filePath)
	handleError(err)
	defer file.Close()

	var list []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		list = append(list, scanner.Text())
	}

	handleError(scanner.Err())

	return list
}

func dumpSnapshots(filePath string, snapshots []ProcessResult) {
	file, err := os.Create(filePath)
	handleError(err)
	defer file.Close()

	writeIntField(file, "count", len(snapshots))
	for _, snapshot := range snapshots {
		writeBlobField(file, "shell", []byte(snapshot.Shell))
		writeIntField(file, "returncode", snapshot.ReturnCode)
		writeBlobField(file, "stdout", snapshot.Stdout)
		writeBlobField(file, "stderr", snapshot.Stderr)
	}
}

func loadSnapshots(filePath string) []ProcessResult {
	var snapshots []ProcessResult
	file, err := os.Open(filePath)
	handleError(err)
	reader := bufio.NewReader(file)

	count, err := readIntField(reader, []byte("count"))
	handleError(err)
	for i := 0; i < count; i += 1 {
		shell, err := readBlobField(reader, []byte("shell"))
		handleError(err)
		returnCode, err := readIntField(reader, []byte("returncode"))
		handleError(err)
		stdout, err := readBlobField(reader, []byte("stdout"))
		handleError(err)
		stderr, err := readBlobField(reader, []byte("stderr"))
		handleError(err)

		snapshots = append(snapshots, ProcessResult{
			Shell:      string(shell),
			ReturnCode: returnCode,
			Stdout:     stdout,
			Stderr:     stderr,
		})
	}
	return snapshots
}

func record(programName string, subCommand string, args []string, jobs int) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "%s %s <test.list>\n", programName, subCommand)
		fmt.Fprintln(os.Stderr, "ERROR: no test.list is provided")
		os.Exit(1)
	}
	testListPath := args[0]
	shells := loadList(testListPath)
	snapshots := make([]ProcessResult, len(shells))
	if jobs > 0 {
		var wg sync.WaitGroup
		var mu sync.Mutex
		semaphore := make(chan struct{}, jobs)
		for i, shell := range shells {
			wg.Add(1)
			go capture(snapshots, i, shell, &wg, &mu, semaphore)
		}
		wg.Wait()
	} else {
		for i, shell := range shells {
			capture(snapshots, i, shell, nil, nil, nil)
		}
	}
	dumpSnapshots(fmt.Sprintf("%s.bi", testListPath), snapshots)
}

func replaying(shell string, snapshot ProcessResult, programName string, testListPath string, wg *sync.WaitGroup, semaphore chan struct{}) {
	if wg != nil {
		defer wg.Done()
	}
	if semaphore != nil {
		semaphore <- struct{}{}
	}

	snapshotShell := snapshot.Shell
	fmt.Printf("REPLAYING: %s\n", shell)
	if shell != snapshotShell {
		fmt.Fprintln(os.Stderr, "UNEXPECTED: shell command")
		fmt.Fprintf(os.Stderr, "    EXPECTED: %s", snapshotShell)
		fmt.Fprintf(os.Stderr, "    ACTUAL:   %s", shell)
		fmt.Fprintf(os.Stderr, "NOTE: You may want to do `%s record %s` to update %s.bi", programName, testListPath, testListPath)
		if semaphore != nil {
			<-semaphore
		}
		return
	}
	args := strings.Split(shell, " ")
	cmd := exec.Command(args[0], args[1:]...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
		}
	}

	if exitCode != snapshot.ReturnCode {
		fmt.Fprintf(os.Stderr, "UNEXPECTED: return code in %s\n", shell)
		fmt.Fprintf(os.Stderr, "    EXPECTED: %d\n", snapshot.ReturnCode)
		fmt.Fprintf(os.Stderr, "    ACTUAL: %d\n", exitCode)
	}

	if bytes.Compare(stdout.Bytes(), snapshot.Stdout) != 0 {
		fmt.Fprintf(os.Stderr, "UNEXPECTED: stdout in %s\n", shell)
		fmt.Fprintf(os.Stderr, "    EXPECTED: %s\n", snapshot.Stdout)
		fmt.Fprintf(os.Stderr, "    ACTUAL: %s\n", stdout.Bytes())
	}

	if bytes.Compare(stderr.Bytes(), snapshot.Stderr) != 0 {
		fmt.Fprintf(os.Stderr, "UNEXPECTED: stderr in %s\n", shell)
		fmt.Fprintf(os.Stderr, "    EXPECTED: %s\n", snapshot.Stderr)
		fmt.Fprintf(os.Stderr, "    ACTUAL: %s\n", stderr.Bytes())
	}

	if semaphore != nil {
		<-semaphore
	}
}

func replay(programName string, subCommand string, args []string, jobs int) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s %s <test.list>\n", programName, subCommand)
		fmt.Fprintln(os.Stderr, "ERROR: no test.list is provided")
		os.Exit(1)
	}
	testListPath := args[0]

	shells := loadList(testListPath)
	snapshots := loadSnapshots(fmt.Sprintf("%s.bi", testListPath))

	if len(shells) != len(snapshots) {
		fmt.Printf("UNEXPECTED: Amount of shell commands in f%s\n", testListPath)
		fmt.Printf("    EXPECTED: %d\n", len(snapshots))
		fmt.Printf("    ACTUAL:   %d\n", len(shells))
		fmt.Printf("NOTE: You may want to do `%s record %s` to update %s.bi\n", programName, testListPath, testListPath)
		os.Exit(1)
	}
	if jobs > 0 {
		var wg sync.WaitGroup
		semaphore := make(chan struct{}, jobs)
		for i := range len(shells) {
			wg.Add(1)
			go replaying(shells[i], snapshots[i], programName, testListPath, &wg, semaphore)
		}
		wg.Wait()
	} else {
		for i := range len(shells) {
			replaying(shells[i], snapshots[i], programName, testListPath, nil, nil)
		}
	}
	fmt.Println("OK")
}

func main() {
	programName := os.Args[0]
	args := os.Args[1:]
	jobs := 0

	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s [-j N] <record|replay> <test.list>\n", programName)
		fmt.Fprintln(os.Stderr, "ERROR: no subcommand is provided")
		os.Exit(1)
	}

	var subCommand string
	if args[0] == "-j" {
		var err error
		jobs, err = strconv.Atoi(args[1])
		handleError(err)
		subCommand = args[2]
		args = args[3:]
	} else if args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintf(os.Stderr, "Usage: %s [-j N] <record|replay> <test.list>\n", programName)
		return
	} else {
		subCommand = args[0]
		args = args[1:]
	}

	switch subCommand {
	case "record":
		record(programName, subCommand, args, jobs)
	case "replay":
		replay(programName, subCommand, args, jobs)
	default:
		fmt.Fprintf(os.Stderr, "ERROR: unknown subcommand %s\n", subCommand)
		os.Exit(1)
	}
}
