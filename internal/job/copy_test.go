package job

import "testing"

func TestSplitRemoteWindowsDrive(t *testing.T) {
	tg, rem := splitRemote(`C:\tmp\a`)
	if tg != "" || rem != `C:\tmp\a` {
		t.Fatalf("%q %q", tg, rem)
	}
	tg, rem = splitRemote("dev-web:/tmp/a")
	if tg != "dev-web" || rem != "/tmp/a" {
		t.Fatalf("%q %q", tg, rem)
	}
}

func TestParseCopy(t *testing.T) {
	s, err := parseCopy(`C:\tmp\a`, "dev-web:/tmp/a")
	if err != nil {
		t.Fatal(err)
	}
	if s.Direction != "upload" || s.Target != "dev-web" {
		t.Fatalf("%+v", s)
	}
	s, err = parseCopy(`C:\tmp\a`, "t:/tmp/a")
	if err != nil {
		t.Fatal(err)
	}
	if s.Direction != "upload" || s.Target != "t" {
		t.Fatalf("one-letter target: %+v", s)
	}
	_, err = parseCopy("a", "b")
	if err == nil {
		t.Fatal("expected invalid")
	}
}
