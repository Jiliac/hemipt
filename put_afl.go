package main

import (
	"fmt"
	"log"

	"encoding/binary"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// *****************************************************************************
// ******************************* Run Inputs **********************************

var (
	helloChildA = [4]byte{0, 0, 0, 0}
	helloChildS = helloChildA[:]
)

type runMeta struct {
	sig     syscall.Signal
	status  syscall.WaitStatus
	crashed bool
	hanged  bool

	trace []byte // Only used if is fit.
}

func (put *aflPutT) run(testCase []byte) (runInfo runMeta, err error) {
	zeroShm(put.trace)

	if len(testCase) > 0 {
		_, err = put.writer.Write(testCase)
		if err != nil {
			log.Printf("Could not write testCase: %v\n", err)
			return runInfo, err
		}
	} else {
		fmt.Println("Empty testCase.")
	}

	// Start running
	_, err = put.ctlPipeW.Write(helloChildS)
	if err != nil {
		log.Printf("Problem when writing in control pipe: %v\n", err)
		return runInfo, err
	}
	encodedWorkpid := make([]byte, 4)
	_, err = put.stPipeR.Read(encodedWorkpid)
	if err != nil {
		log.Printf("Problem when reading the status pipe: %v\n", err)
		return runInfo, err
	}
	pid := int(binary.LittleEndian.Uint32(encodedWorkpid))

	// Start run.
	encodedStatus := make([]byte, 4)
	reportChan := make(chan error)
	timer := time.NewTimer(put.timeout)
	go func() {
		_, stErr := put.stPipeR.Read(encodedStatus)
		reportChan <- stErr
	}()

	// Wait for result
	select {
	case err = <-reportChan:
		timer.Stop()
	case <-timer.C:
		p, errP := os.FindProcess(pid)
		if errP != nil {
			log.Printf("Could find child process run (pid=%d): %v.\n", pid, errP)
		} else {
			errP = p.Kill()
			if errP != nil {
				log.Printf("Could not kill process (pid=%d): %v.\n", pid, errP)
			} else {
				runInfo.hanged = true
			}
		}
	}

	if err != nil {
		log.Printf("Problem while reading status: %v.\n", err)
	}

	status := binary.LittleEndian.Uint32(encodedStatus)
	runInfo.status = syscall.WaitStatus(status)
	if stat := syscall.WaitStatus(status); stat.Signaled() {
		runInfo.crashed = true
		runInfo.sig = stat.Signal()
	} else if put.usesMsan && stat.ExitStatus() == msanError {
		runInfo.crashed = true
	}

	return runInfo, err
}

// *****************************************************************************
// ********************************* Setup *************************************

const (
	mapSizePow2 = 16
	mapSize     = 1 << mapSizePow2

	ipcPrivate = 0
	ipcCreat   = 0x200
	ipcExcl    = 0x400
	ipcRmid    = 0

	forksrvFd = 198

	// Memory Sanitizer configuration usage, from AFL:
	// "MSAN is tricky, because it doesn't support abort_on_error=1 at this
	// point. So, we do this in a very hacky way."
	// Meaning, defines a signal that will be sent when security policy of MSAN
	// is activated, and we catch that.
	msanError = 86

	shmEnvVar        = "__AFL_SHM_ID"
	persistentEnvVar = "__AFL_PERSISTENT"
	deferEnvVar      = "__AFL_DEFER_FORKSRV"
	asanVar          = "ASAN_OPTIONS"
	msanVar          = "MSAN_OPTIONS"

	persistentSig = "##SIG_AFL_PERSISTENT##"
	deferSig      = "##SIG_AFL_DEFER_FORKSRV##"
	asanDetect    = "libasan.so"
	msanDetect    = "__msan_init"
)

type aflPutT struct {
	trace []byte

	// Used at each run
	writer  putWriter
	timeout time.Duration

	// System
	pid               int
	shmID             uintptr
	usesMsan          bool
	ctlPipeW, stPipeR *os.File
}

type putWriter interface {
	io.Writer
	clean()
}

func startAFLPUT(binPath string, cliArgs []string, timeout time.Duration) (
	put *aflPutT, ok bool) {

	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return
	}
	put = new(aflPutT)

	// ** I - Prepare I/O **
	var okPW, okFrk bool
	var files []uintptr
	fileIn, args, fileArgs, filePathPos := parseArgs(cliArgs)
	if fileIn {
		cliArgs = args
		okPW, put.writer, files = makeFilePUTWriter(args, fileArgs, filePathPos)
	} else { // Input written in stdin.
		okPW, put.writer, files = makeStdinPUTWriter()
	}
	if !okPW {
		return
	}

	// ** II - Prepare binary launch **
	okShm, shmID, trace := setupShm()
	if !okShm {
		return
	}
	put.trace, put.shmID = trace, shmID
	env := os.Environ()
	var extraEnv []string
	extraEnv, put.usesMsan = getExtraEnvs(binPath, shmID)
	env = append(env, extraEnv...)
	procAttr := &syscall.ProcAttr{
		Env:   env,
		Files: files,
		Sys:   &syscall.SysProcAttr{Setsid: true},
	}

	// ** III - Launch binary **
	okFrk, put.ctlPipeW, put.stPipeR, put.pid = initForkserver(binPath, cliArgs, procAttr)
	if !okFrk {
		return
	}

	put.timeout = timeout
	ok = true

	return put, ok
}

func getExtraEnvs(binPath string, shmID uintptr) (envs []string, usesMsan bool) {
	binContent, err := ioutil.ReadFile(binPath)
	if err != nil {
		log.Fatalf("Couldn't open the binary: %v.\n", err)
	}

	// Shared memory with the PUT to get the branch hit count.
	reShm := regexp.MustCompile(shmEnvVar)
	if !reShm.Match(binContent) {
		log.Fatal("This binary wasn't instrumented correctly.")
	}
	envs = append(envs, fmt.Sprintf("%s=%d", shmEnvVar, shmID))
	//
	// Persistent mode
	rePer := regexp.MustCompile(persistentSig)
	if rePer.Match(binContent) {
		fmt.Println("Persistent mode detected.")
		envs = append(envs, fmt.Sprintf("%s=1", persistentEnvVar))
	}
	//
	// Deferred fork server
	reDef := regexp.MustCompile(deferSig)
	if reDef.Match(binContent) {
		fmt.Println("Deferred fork server detected.")
		envs = append(envs, fmt.Sprintf("%s=1", deferEnvVar))
	}

	// Address and Memory SANitizers
	reASAN := regexp.MustCompile(asanDetect)
	reMSAN := regexp.MustCompile(msanDetect)
	isAsan, isMsan := reASAN.Match(binContent), !reMSAN.Match(binContent)
	if !isAsan && !isMsan {
		return envs, usesMsan
	} else if isMsan {
		usesMsan = true
	}
	//
	// ASAN
	asanOps, ok := os.LookupEnv(asanVar)
	if ok {
		if !regexp.MustCompile("abort_on_error=1").MatchString(asanOps) {
			log.Fatal("Custom ASAN_OPTIONS set without abort_on_error=1 - please fix!")
		} else if !regexp.MustCompile("symbolize=0").MatchString(asanOps) {
			log.Fatal("Custom ASAN_OPTIONS set without symbolize=0 - please fix!")
		}
	} else {
		envs = append(envs, fmt.Sprintf("%s=abort_on_error=1:detect_leaks=0:"+
			"symbolize=0:allocator_may_return_null=1", asanVar))
	}
	// MSAN
	ec := fmt.Sprintf("exit_code=%d", msanError)
	msanOps, ok := os.LookupEnv(msanVar)
	if ok {
		if !regexp.MustCompile(ec).MatchString(msanOps) {
			log.Fatalf("Custom MSAN_OPTIONS set without %s - please fix!\n", ec)
		} else if !regexp.MustCompile("symbolize=0").MatchString(msanOps) {
			log.Fatal("Custom MSAN_OPTIONS set without symbolize=0 - please fix!")
		}
	} else {
		envs = append(envs, fmt.Sprintf("%s=%s:symbolize=0:abort_on_error=1:"+
			"allocator_may_return_null=1:msan_track_origins=0", msanVar, ec))
	}

	return envs, usesMsan
}

func (put *aflPutT) clean() {
	killAllChildren(put.pid)
	proc, err := os.FindProcess(put.pid)
	if err != nil {
		log.Printf("Could not get fork server process %d: %v.\n", put.pid, err)
	} else {
		err = proc.Kill()
		if err != nil {
			log.Printf("Could not kill fork server: %v.\n", err)
		}
	}

	closeShm(put.shmID)
	put.writer.clean()
}

func parseArgs(cliArgs []string) (
	fileIn bool, args []string, fileArg int, filePathPos [2]int) {

	// 1. Make a deep copy of argument list.
	args = make([]string, len(cliArgs))
	for i, a := range cliArgs {
		tmp := make([]byte, len(a))
		copy(tmp, []byte(a))
		args[i] = string(tmp)
	}

	// 2. Check if as the "magic" file sequence.
	re := regexp.MustCompile("(@*)+@@")
	for i, a := range args {
		res := re.FindAllStringIndex(a, -1)
		if len(res) == 0 {
			continue
		}

		fileIn = true
		fileArg = i
		pos := res[len(res)-1]
		filePathPos[1] = pos[1]
		filePathPos[0] = filePathPos[1] - 2
		return
	}

	return
}

// ********************
// **** PUT Writer ****

var devNull *os.File

func init() {
	var err error
	devNull, err = os.OpenFile(os.DevNull, os.O_RDWR, 0666)
	if err != nil {
		log.Fatalf("Could not open pipes /dev/null: %v.\n", err)
	}
}

type fileIO struct {
	path string
}

func (fio fileIO) Write(tc []byte) (n int, err error) {
	err = os.Remove(fio.path)
	if err != nil {
		log.Printf("Problem removing test case path: %v "+
			"(normal if first time running).\n", err)
	}
	f, err := os.OpenFile(fio.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		log.Printf("Could not open test case file %s: %v.\n", fio.path, err)
	}
	n, err = f.Write(tc)
	if err != nil {
		return n, err
	}
	err = f.Close()
	return n, err
}
func (fio fileIO) clean() {}

func makeFilePUTWriter(args []string, fileArg int, filePathPos [2]int) (
	ok bool, pw putWriter, files []uintptr) {

	// Setup
	fileInName := filepath.Join(workDir, fmt.Sprintf("tmp-%x", rand.Int63()))
	ok, pw = true, fileIO{path: fileInName}
	files = []uintptr{devNull.Fd(), devNull.Fd(), devNull.Fd()}

	// Prepare argument(s)
	var newArg []byte
	arg := args[fileArg]
	if filePathPos[0] > 0 {
		newArg = make([]byte, filePathPos[0])
		copy(newArg, []byte(arg[:filePathPos[0]]))
	}
	newArg = append(newArg, []byte(fileInName)...)
	if filePathPos[1] != len(arg) {
		newArg = append(newArg, []byte(arg[filePathPos[1]:])...)
	}
	args[fileArg] = string(newArg)

	return ok, pw, files
}

// **************************

type stdinIO struct{ *os.File }

func (sio stdinIO) Write(tc []byte) (n int, err error) {
	_, err = sio.File.Seek(0, os.SEEK_SET) // Reset head from last read.
	if err != nil {
		log.Printf("Error moving head of stdin writing: %v.\n", err)
	}
	n, err = sio.File.Write(tc)
	if err != nil {
		return n, err
	}
	err = sio.File.Truncate(int64(n)) // To ensure no "spill over" from last write.
	if err != nil {
		return n, err
	}
	_, err = sio.File.Seek(0, os.SEEK_SET) // To prepare the PUT reading.
	return n, err
}
func (sio stdinIO) clean() {
	name := sio.Name()
	errClose := sio.Close()
	if errClose != nil {
		log.Printf("Problem closing previous file descriptor: %v.\n", errClose)
	}
	err := os.Remove(name)
	if err != nil {
		log.Printf("Could not close input file: %v\n", err)
	}
}

func makeStdinPUTWriter() (ok bool, pw putWriter, files []uintptr) {
	fileInName := filepath.Join(workDir, fmt.Sprintf("tmp-%x", rand.Int63()))
	// Need to use the system call directly because std library use O_CLOEXEC
	// making impossible to pass this file to child.
	fd, err := syscall.Open(fileInName, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		log.Printf("Could not open %s: %v\n", fileInName, err)
		return
	}
	f := os.NewFile(uintptr(fd), fileInName)
	//
	ok, pw = true, stdinIO{File: f}
	files = []uintptr{f.Fd(), devNull.Fd(), devNull.Fd()}
	return ok, pw, files
}

// ***********************
// **** Shared Memory ****

func setupShm() (ok bool, id uintptr, trace []byte) {
	var err syscall.Errno
	id, _, err = syscall.RawSyscall(syscall.SYS_SHMGET, ipcPrivate, mapSize,
		ipcCreat|ipcExcl|0600)
	if err != 0 {
		log.Printf("Problem creating a new shared memory segment: %v\n", err)
		return
	}

	segMap, _, err := syscall.RawSyscall(syscall.SYS_SHMAT, id, 0, 0)
	if err != 0 || id < 0 {
		log.Printf("Problem attaching segment: %v\n", err)
		return
	}

	// Dirty thing we have to do (to use AFL instrumentation).
	traceBitPt := (*[mapSize]byte)(unsafe.Pointer(segMap))

	ok = true
	trace = (*traceBitPt)[:]
	return ok, id, trace
}

func closeShm(id uintptr) {
	_, _, err := syscall.RawSyscall(syscall.SYS_SHMCTL, id, ipcRmid, 0)
	if err != 0 {
		log.Fatalf("Problem closing shared memory segment: %v\n", err)
	}
}

func zeroShm(traceBitPt []byte) {
	for i := range traceBitPt {
		traceBitPt[i] = 0
	}
}

// ********************************
// ** Fork Server Initialization **
// Binary launch specific to AFL.

func initForkserver(binPath string, cliArgs []string, procAttr *syscall.ProcAttr) (
	ok bool, ctlPipeW *os.File, stPipeR *os.File, pid int) {

	// ** I - Pipe Management **
	// AFL fork server pipes.
	// The "control" pipe tell the fork server to prepare to receive an input.
	// The "status" pipe read the status answer from the input run.
	// See "man wait" for a description of the status.
	//
	// I.a Create the pipes.
	var ctlPipe, stPipe [2]int
	err1 := syscall.Pipe(ctlPipe[0:])
	err2 := syscall.Pipe(stPipe[0:])
	if err1 != nil || err2 != nil {
		log.Printf("Error while creating pipes: (ctl) %v - (st) %v\n",
			err1, err2)
		return
	}
	//
	ctlPipeR, stPipeW := ctlPipe[0], stPipe[1] // Just renaming.
	ctlPipeW = os.NewFile(uintptr(ctlPipe[1]), "|1")
	stPipeR = os.NewFile(uintptr(stPipe[0]), "|0")
	//
	// I.b Move the pipe where AFL-instrumented binaries expect them.
	err1 = syscall.Dup2(ctlPipeR, forksrvFd)
	err2 = syscall.Dup2(stPipeW, forksrvFd+1)
	if err1 != nil || err2 != nil {
		log.Printf("Error while dup2-licating pipes: (ctl) %v - (st) %v\n",
			err1, err2)
		return
	}
	//
	err1 = syscall.Close(ctlPipeR)
	err2 = syscall.Close(stPipeW)
	if err1 != nil || err2 != nil {
		log.Printf("Error while closing unused pipes: (ctl) %v - (st) %v\n",
			err1, err2)
		return
	}
	//
	// I.c Flag the pipes so they aren't visible from the fork server (and only
	// from "here").
	_, _, errno1 := syscall.RawSyscall(syscall.SYS_FCNTL, ctlPipeW.Fd(),
		syscall.F_SETFD, syscall.FD_CLOEXEC)
	_, _, errno2 := syscall.RawSyscall(syscall.SYS_FCNTL, stPipeR.Fd(),
		syscall.F_SETFD, syscall.FD_CLOEXEC)
	if errno1 != 0 || errno2 != 0 {
		log.Printf("Problem setting ctlPipeW and stPipeR flag to CLOEXEC."+
			" errno: (ctlPipeW) %d (stPipeR) %d\n", errno1, errno2)
		return
	}

	// ** II - (Finally) Fork **
	var err error
	execArgs := append([]string{binPath}, cliArgs...)
	pid, err = syscall.ForkExec(binPath, execArgs, procAttr)
	if err != nil {
		log.Printf("Couldn't ForkExec %s: %v.\n", binPath, err)
		return
	}
	//
	// Fork epilogue: closing the pipes only the child is supposed to write into.
	err1 = syscall.Close(forksrvFd)
	err2 = syscall.Close(forksrvFd + 1)
	if err1 != nil || err2 != nil {
		log.Printf("Error while closing the fork server (main process) pipes:"+
			"(ctl) %v - (st) %v\n", err1, err2)
	}

	// ** III - Test **
	// (Actually, this first handshake is needed to setup the fork server correctly.)
	timer := time.NewTimer(time.Second)
	encodedStatus := make([]byte, 4)
	reportChan := make(chan error)
	go func() { _, errR := stPipeR.Read(encodedStatus); reportChan <- errR }()
	select {
	case err = <-reportChan:
		timer.Stop()
	case <-timer.C:
		errString := fmt.Sprintf("child (pid=%d) hanged", pid)
		err = fmt.Errorf(errString)
	}
	if err != nil {
		log.Printf("Test handshake w/ frksrv failed: %v.\n", err)
		return
	}

	status := binary.LittleEndian.Uint32(encodedStatus)
	if stat := syscall.WaitStatus(status); stat.Signaled() {
		fmt.Printf("stat.Signal() = %+v\n", stat.Signal())
	} else {
		ok = true
	}

	return ok, ctlPipeW, stPipeR, pid
}

// ***************************
// ** Kill Process Children **
func killAllChildren(pid int) {
	children := listChildren(pid)
	for _, childPid := range children {
		killAllChildren(childPid)
		proc, err := os.FindProcess(childPid)
		if err != nil {
			log.Printf("Could not find child proc (pid=%d): %v.\n", childPid, err)
			continue
		}
		proc.Kill() // Don't care much if it fails...
	}
}

func listChildren(pid int) (childrenPids []int) {
	pidStr := fmt.Sprintf("%d", pid)
	childrenPath := filepath.Join("/proc", pidStr, "task", pidStr, "children")
	childrenStr, err := ioutil.ReadFile(childrenPath)
	if err != nil {
		log.Printf("Could not read children of %d: %v.\n", pid, err)
		return
	}

	childList := strings.Split(string(childrenStr), " ")
	for _, child := range childList {
		if len(child) == 0 {
			continue
		}
		childPid, err := strconv.Atoi(child)
		if err != nil {
			log.Print(err)
			continue
		}
		childrenPids = append(childrenPids, childPid)
	}

	return childrenPids
}

// *****************************************************************************
// *************************** CPU Affinity Managing ***************************

const deactivateHyperthread = true

var getCPUMtx sync.Mutex

func lockRoutine() (bool, int) {
	getCPUMtx.Lock()
	defer getCPUMtx.Unlock()
	unusedCPUs := getUnusedCPUs()

	runtime.LockOSThread()

	targetedCPU := -1
	if len(unusedCPUs) > 0 {
		for cpu, ok := range unusedCPUs {
			if !ok {
				continue
			}
			targetedCPU = cpu
			break
		}

	} else { // No CPU available.
		log.Print("No CPU available.")
		return false, -1
	}

	var set unix.CPUSet
	set.Zero()
	set.Set(targetedCPU)

	err := unix.SchedSetaffinity(0, &set)
	if err != nil {
		log.Printf("Could not associate PUT with a CPU: %v.\n", err)
	}
	return true, targetedCPU
}

func getUnusedCPUs() (unusedCPUs []bool) {
	nbCPU := runtime.NumCPU()
	//
	unusedCPUs = make([]bool, nbCPU)
	for cpu := range unusedCPUs {
		unusedCPUs[cpu] = true
	}
	if deactivateHyperthread {
		for cpu := range unusedCPUs {
			if cpu%2 == 1 {
				unusedCPUs[cpu] = false
			}
		}
	}

	procDir, err := ioutil.ReadDir("/proc")
	if err != nil {
		log.Printf("Could not read /proc: %v.\n", err)
		return
	}

	for _, procFileInfo := range procDir {
		if !procFileInfo.IsDir() { // Only care about dirs
			continue
		}
		name := procFileInfo.Name()
		if name[0] < '0' || name[0] > '9' { // Only care about pids
			continue
		}

		pid, err := strconv.Atoi(name)
		if err != nil {
			continue
		}

		var set unix.CPUSet
		set.Zero()
		err = unix.SchedGetaffinity(pid, &set)
		if err != nil {
			continue
		}

		count := set.Count()
		if count == nbCPU {
			continue
		}
		status, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err != nil {
			log.Printf("Cannot read %s status: %v.\n", name, err)
			continue
		}
		if !strings.Contains(string(status), "VmSize") { // Prob' kernel task
			continue
		}

		for cpu := range unusedCPUs {
			if set.IsSet(cpu) {
				unusedCPUs[cpu] = unusedCPUs[cpu] && false
			}
		}
	}

	return unusedCPUs
}
