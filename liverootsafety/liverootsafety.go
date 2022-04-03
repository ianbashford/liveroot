package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"syscall"
)

type flags struct {
	dryrun bool
}

var (
	fargs = flags{}
)

func init() {
	flag.BoolVar(&fargs.dryrun, "dryrun", false, "Dry run - print output but don't unmount anything")
}

const (
	procmountsFile = "/proc/mounts"
	motdFile       = "/etc/motd"
	rootFileSystem = "/"
	bootFileSystem = "/boot"
	zramFileSystem = "zram"
)

// Mount is a structure used to contain mount point data
type Mount struct {
	Device         string
	MountPoint     string
	FileSystemType string
	Flags          string
	Bsize          int64
	Blocks         uint64
	Total          uint64
	Used           uint64
	Avail          uint64
	PCT            uint8
}

type mountinfomap map[string]Mount

// mountinfo returns a map of mounts representing
// the data in /proc/mounts
func mountinfo() (mountinfomap, error) {
	buf, err := os.ReadFile(procmountsFile)
	if err != nil {
		return nil, err
	}
	return mountinfoFromBytes(buf)
}

// returns a map generated from the bytestream returned
// from /proc/mounts
// for tidiness, we decide to ignore filesystems of size 0
// to exclude cgroup, procfs and sysfs types
func mountinfoFromBytes(buf []byte) (mountinfomap, error) {
	ret := make(mountinfomap)
	for _, line := range bytes.Split(buf, []byte{'\n'}) {
		kv := bytes.SplitN(line, []byte{' '}, 6)
		if len(kv) != 6 {
			// can't interpret this
			continue
		}
		key := string(kv[1])
		var mnt Mount
		mnt.Device = string(kv[0])
		mnt.MountPoint = string(kv[1])
		mnt.FileSystemType = string(kv[2])
		mnt.Flags = string(kv[3])
		if mnt.MountPoint == rootFileSystem || mnt.MountPoint == bootFileSystem {
			ret[key] = mnt
		}
	}
	return ret, nil
}

func unmount_boot() error {
	err := syscall.Unmount(bootFileSystem, 0)
	return err
}

func write_motd(inRam bool, bootMounted bool, rootFs string) error {
	var motdMessage []byte
	if !inRam {
		motdMessage = []byte("RootFS mounted on " + rootFs)
	} else if inRam && !bootMounted {
		motdMessage = []byte("RootFS mounted on " + rootFs)
	} else {
		motdMessage = []byte("RootFS mounted on " + rootFs + " but /boot still mounted.")
	}
	err := os.WriteFile(motdFile, motdMessage, 0644)
	return err
}

func process(w io.Writer, fargs flags, args []string) error {
	mounts, err := mountinfo()
	if err != nil {
		return fmt.Errorf("mountinfo()=_,%q, want: _,nil", err)
	}
	rootMnt, _ := mounts[rootFileSystem]
	rootInRam := strings.Contains(rootMnt.Device, zramFileSystem)
	_, ok := mounts[bootFileSystem]
	if ok {
		// /boot is mounted
		// unmount it if we have a ramroot
		if rootInRam && !fargs.dryrun {
			unmount_err := unmount_boot()
			if unmount_err != nil {
				fmt.Fprintf(os.Stderr, "Unmounting %s failed\n", bootFileSystem)
				fmt.Fprintln(os.Stderr, unmount_err.Error())
			} else {
				ok = false
				fmt.Printf("/boot umounted by liverootsafety")
			}
		} else {
			fmt.Printf("/boot not umounted by liverootsafety")
		}
	}
	motd_err := write_motd(rootInRam, ok, rootMnt.Device)
	if motd_err != nil {
		fmt.Printf("Writing MOTD file failed\n%s", motd_err.Error())
	}
	return nil
}

func main() {
	flag.Parse()
	if err := process(os.Stdout, fargs, flag.Args()); err != nil {
		log.Fatal(err)
	}
}
