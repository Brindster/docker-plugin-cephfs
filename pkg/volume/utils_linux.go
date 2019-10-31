package volume

import "syscall"

func (_ fsMounter) Mount(source string, target string, fstype string, data string) error {
	return syscall.Mount(source, target, fstype, 0, data)
}

func (_ fsMounter) Unmount(target string) error {
	return syscall.Unmount(target, 0)
}
