package apt

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"testing"
)

func TestDropPrivilegesNoop(t *testing.T) {
	for name, config := range map[string]Config{
		"unset": {},
		"root":  {"apt::sandbox::user": "root"},
		// the user is not even looked up
		"Debug::NoDropPrivs": {"apt::sandbox::user": "no-such-user", "debug::nodropprivs": "yes"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := DropPrivileges(config); err != nil {
				t.Fatal(err)
			}
		})
	}
	if os.Getuid() != 0 {
		if err := DropPrivileges(Config{"apt::sandbox::user": "no-such-user"}); err != nil {
			t.Fatal(err)
		}
	}
}

// TestDropPrivileges drops to nobody in a child process.
func TestDropPrivileges(t *testing.T) {
	if os.Getenv("APT_TRANSPORT_OCI_TEST_DROP") == "1" {
		if err := DropPrivileges(Config{"apt::sandbox::user": "nobody"}); err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
		groups, _ := os.Getgroups()
		fmt.Printf("uid=%d gid=%d groups=%v\n", os.Getuid(), os.Getgid(), groups)
		os.Exit(0)
	}
	if os.Getuid() != 0 {
		t.Skip("needs root")
	}
	u, err := user.Lookup("nobody")
	if err != nil {
		t.Skip(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestDropPrivileges$")
	cmd.Env = append(os.Environ(), "APT_TRANSPORT_OCI_TEST_DROP=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if want := fmt.Sprintf("uid=%s gid=%s groups=[%s]", u.Uid, u.Gid, u.Gid); !strings.Contains(string(out), want) {
		t.Fatalf("expected %q, got %q", want, out)
	}
}
