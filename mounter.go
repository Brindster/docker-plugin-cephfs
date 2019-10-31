// +build !linux

package main

import (
	"fmt"
	"os/exec"
)

func (_ fsMounter) Mount(source string, target string, fstype string, data string) error {
	cmd := exec.Command("mount", "-t", fstype, "-o", data, source, target)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error mounting: %s\ncommand output: %s", err, out)
	}

	return nil
}

func (_ fsMounter) Unmount(target string) error {
	cmd := exec.Command("umount", target)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error mounting: %s\ncommand output: %s", err, out)
	}

	return nil
}
