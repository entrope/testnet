package main

import (
	"testing"
)

func TestSplitLineEmpty(t *testing.T) {
	argv := ScriptSplitLine("")
	if argv != nil {
		t.Errorf(`ScriptSplitLine("") = %v; want nil`, argv)
	}
}

func TestSplitLineSpaces(t *testing.T) {
	argv := ScriptSplitLine(" \n")
	if argv != nil {
		t.Errorf(`ScriptSplitLine(" \n") = %v; want nil`, argv)
	}
}

func TestSplitLineComment(t *testing.T) {
	argv := ScriptSplitLine("# hello world\r\n")
	if argv != nil {
		t.Errorf(`ScriptSplitLine("# hello world\r\n") = %v; want nil`, argv)
	}
}

func TestSplitLineLabelOnly(t *testing.T) {
	defer func() {
		recover()
	}()
	argv := ScriptSplitLine(":Joe")
	t.Errorf(`ScriptSplitLine(":Joe") = %v; want panic`, argv)
}

func TestSplitLineLabeled(t *testing.T) {
	argv := ScriptSplitLine(":Joe SCHMOE :world\r\n")
	if len(argv) != 3 || argv[0] != "SEND" || argv[1] != "Joe" || argv[2] != "SCHMOE :world" {
		t.Errorf(`ScriptSplitLine(":Joe SCHMOE :world\r\n") = %v; want `+
			`{ "SEND", "Joe", "SCHMOE :world" }`, argv)
	}
}

func TestSplitLineSourced(t *testing.T) {
	argv := IrcSplitLine(":Joe SCHMOE :world\r\n")
	if len(argv) != 3 || argv[0] != "Joe" || argv[1] != "SCHMOE" || argv[2] != "world" {
		t.Errorf(`IrcSplitLine(":Joe SCHMOE :world\r\n") = %v; want `+
			`{ "Joe", "SCHMOE", "world" }`, argv)
	}
}

func TestSplitLineIRC(t *testing.T) {
	argv := IrcSplitLine("HELLO :world\r\n")
	if len(argv) != 3 || argv[0] != "" || argv[1] != "HELLO" || argv[2] != "world" {
		t.Errorf(`IrcSplitLine("HELLO :world\r\n") = %v; want `+
			`{ "", "HELLO", "world" }`, argv)
	}
}

func TestSplitLinePlain(t *testing.T) {
	argv := ScriptSplitLine("SEND Joe SCHMOE\n")
	if len(argv) != 3 || argv[0] != "SEND" || argv[1] != "Joe" || argv[2] != "SCHMOE" {
		t.Errorf(`ScriptSplitLine("SEND Joe SCHMOE\n") = %v; want `+
			`{ "SEND", "Joe", "SCHMOE" }`, argv)
	}
}

func TestSplitLineColon(t *testing.T) {
	argv := ScriptSplitLine("TEST :With spaces")
	if len(argv) != 2 || argv[0] != "TEST" || argv[1] != "With spaces" {
		t.Errorf(`ScriptSplitLine("TEST :With spaces") = %v; want`+
			`{ "TEST", "With spaces" }`, argv)
	}
}

func TestSplitLineCommandOnly(t *testing.T) {
	argv := ScriptSplitLine("HELLOWORLD")
	if len(argv) != 1 || argv[0] != "HELLOWORLD" {
		t.Errorf(`ScriptSplitLine("HELLOWORLD") = %v; want`+
			`{ "HELLOWORLD" }`, argv)
	}
}

var addressTests = []struct {
	Address string
	Host    string
	Port    uint16
}{
	{"127.0.0.1:123", "127.0.0.1", 123},
	{"[2001:db8::1]:80", "2001:db8::1", 80},
}

func TestSplitAddress(t *testing.T) {
	for _, ref := range addressTests {
		host, port := SplitAddress(ref.Address)
		if host != ref.Host || port != ref.Port {
			t.Errorf("TestSplitAddress(\"%s\") = %s, %d; want %s, %d\n",
				ref.Address, host, port, ref.Host, ref.Port)
		}
	}
}
