package destructive

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLooksLikeRm(t *testing.T) {
	hits := []string{
		"rm -rf /home/huangyu/sub-2-api",
		"rm file.txt",
		"/bin/rm -rf *",
		"/usr/bin/rmdir emptydir",
		"sudo rm -rf /tmp/a",
		"cd /tmp && rm -rf *",
		"cd /home/huangyu/sub-2-api ; rm -rf * .[!.]*",
		"unlink /tmp/x",
		"find /tmp -delete",
		"xargs rm",
		"Remove-Item -Recurse C:\\temp\\a",
		"$(rm -rf /tmp/x)",
		"printf '%s' " + base64.StdEncoding.EncodeToString([]byte("rm -rf /tmp/x\n")) + " | base64 -d | bash -s",
	}
	for _, s := range hits {
		if !LooksLikeRm(s) {
			t.Fatalf("expected hit: %q", s)
		}
	}
	misses := []string{
		"uname -a",
		"chmod 644 file",
		"echo rm -rf",
		"grep rm file",
		"ls /home/huangyu",
		"git clone https://github.com/huangyu6572/sub2api.git .",
		"mkdir -p ~/sub-2-api",
		"firm_upgrade",
		"alarm.sh",
	}
	for _, s := range misses {
		if LooksLikeRm(s) {
			t.Fatalf("false positive: %q matched %q", s, Match(s))
		}
	}
}

func TestRefuseUnlessConfirmed(t *testing.T) {
	if err := RefuseUnlessConfirmed("ls", false); err != nil {
		t.Fatal(err)
	}
	if err := RefuseUnlessConfirmed("rm -rf /tmp/x", true); err != nil {
		t.Fatal(err)
	}
	err := RefuseUnlessConfirmed("rm -rf /tmp/x", false)
	if err == nil || !strings.Contains(err.Error(), "DESTROY_NEEDS_HUMAN") {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "--yes") == false {
		t.Fatal("must say there is no --yes")
	}
}
