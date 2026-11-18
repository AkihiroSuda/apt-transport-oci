package apt

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

// ConfigSandboxUser is the user to drop privileges to.
const ConfigSandboxUser = "APT::Sandbox::User"

// DropPrivileges switches to ConfigSandboxUser when root, as apt's fileutl.cc.
func DropPrivileges(config Config) error {
	name := config.Find(ConfigSandboxUser, "")
	if config.FindB("Debug::NoDropPrivs", false) || name == "" || name == "root" || os.Geteuid() != 0 {
		return nil
	}
	u, err := user.Lookup(name)
	if err != nil {
		return fmt.Errorf("no user %s, can not drop rights: %w", name, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return err
	}
	// Same order as apt; Go applies each to every thread or fails.
	if err := syscall.Setgroups([]int{gid}); err != nil {
		return fmt.Errorf("setgroups: %w", err)
	}
	if err := syscall.Setresgid(gid, gid, gid); err != nil {
		return fmt.Errorf("setresgid: %w", err)
	}
	if err := syscall.Setresuid(uid, uid, uid); err != nil {
		return fmt.Errorf("setresuid: %w", err)
	}
	return nil
}
