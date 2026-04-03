// homard, a milter to add Authentication-Results to self-sent mails
//
// Copyright (C) 2025  Nicolas Peugnet <nicolas@club1.fr>
//
// This program is free software; you can redistribute it and/or
// modify it under the terms of the GNU General Public License
// as published by the Free Software Foundation; either version 2
// of the License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program; if not, see <https://www.gnu.org/licenses/>.

package main

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/emersion/go-milter"
)

func setup(t *testing.T, config string) (string, string, *bytes.Buffer) {
	tmp := t.TempDir()

	// setup logger
	prevLogOut := l.Writer()
	t.Cleanup(func() { l.SetOutput(prevLogOut) })
	r, w := io.Pipe()
	l.SetOutput(w)

	// setup config file
	configPath := filepath.Join(tmp, "homard.conf")
	err := os.WriteFile(configPath, []byte(config), 0664)
	if err != nil {
		t.Fatal(err)
	}
	os.Args = []string{"homard", "-c", configPath}

	// save default conf
	prevConf := conf
	t.Cleanup(func() { conf = prevConf })

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		main()
		wg.Done()
	}()
	t.Cleanup(func() {
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		wg.Wait()
	})

	listener := readListener(t, r)
	buf := &bytes.Buffer{}
	l.SetOutput(buf)
	network, address, _ := strings.Cut(listener, "://")

	return network, address, buf
}

func readListener(t *testing.T, r io.Reader) string {
	scanner := bufio.NewScanner(r)
	read := &bytes.Buffer{}
	for scanner.Scan() {
		line := scanner.Bytes()
		if bytes.HasPrefix(line, []byte("Milter listening")) {
			return string(line[20:])
		}
		read.Write(line)
	}
	t.Fatalf("listener not found, err: %v, read:\n%s", scanner.Err(), read.Bytes())
	return ""
}

func TestMacros(t *testing.T) {
	cases := []struct {
		name     string
		macros   []string
		expected string
	}{
		{
			name:     "basic",
			macros:   []string{"{auth_authen}", "nicolas@club1.fr"},
			expected: "mail.club1.fr; auth=pass smtp.auth=nicolas@club1.fr",
		},
		{
			name:     "login name is not an address",
			macros:   []string{"{auth_authen}", "nicolas"},
			expected: "mail.club1.fr; auth=pass smtp.auth=nicolas",
		},
	}
	config := `
ListenURI = "tcp://127.0.0.1:"
AuthservID = "mail.club1.fr"
`
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testMacros(t, config, c.macros, c.expected, "Authentication-Results field added")
		})
	}
}

func TestConfigNoAuthservID(t *testing.T) {
	config := `
ListenURI = "tcp://127.0.0.1:"
`
	macros := []string{"{auth_authen}", "nicolas@club1.fr"}
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal("unexpected error: ", err)
	}
	expectedField := hostname + "; auth=pass smtp.auth=nicolas@club1.fr"
	testMacros(t, config, macros, expectedField, "Authentication-Results field added")
}

func TestUNIXSocket(t *testing.T) {
	config := `
ListenURI = "unix:///tmp/homard.sock"
AuthservID = "mail.club1.fr"
`
	macros := []string{"{auth_authen}", "nicolas@club1.fr"}
	expectedField := "mail.club1.fr; auth=pass smtp.auth=nicolas@club1.fr"
	testMacros(t, config, macros, expectedField, "Authentication-Results field added")
}

func testMacros(t *testing.T, config string, macros []string, expectedField string, expectedOut ...string) {
	if len(macros)%2 != 0 {
		panic("macros varargs must be pairs")
	}
	network, address, out := setup(t, config)

	client := milter.NewClientWithOptions(network, address, milter.ClientOptions{
		Dialer: &net.Dialer{},
	})
	defer client.Close()
	session, err := client.Session()
	if err != nil {
		t.Fatal("unexpected error: ", err)
	}
	defer session.Close()

	// Send a dummy MAIL FROM, with an authentication macro.
	fromAddr := "nicolas@example.fr"
	err = session.Macros(milter.CodeMail,
		append(macros, "i", "QUEUEID")...,
	)
	if err != nil {
		t.Fatal("unexpected err setting macros: ", err)
	}
	res, err := session.Mail(fromAddr, []string{})
	if err != nil {
		t.Fatal("unexpected err sending MAIL FROM: ", err)
	}
	continueAct := &milter.Action{Code: milter.ActContinue}
	if !reflect.DeepEqual(continueAct, res) {
		t.Errorf("expected %#v, got %#v", continueAct, res)
	}
	for key, val := range map[string]string{
		"From": fromAddr,
	} {
		_, err := session.HeaderField(key, val)
		if err != nil {
			t.Error("unexpected err sending header: ", err)
		}
	}
	_, err = session.HeaderEnd()
	if err != nil {
		t.Error("unexpected err sending EOH: ", err)
	}
	body := bytes.NewReader([]byte("Hello world!"))
	mods, res, err := session.BodyReadFrom(body)
	if err != nil {
		t.Error("unexpected err sending EOB: ", err)
	}
	acceptAct := &milter.Action{Code: milter.ActAccept}
	if !reflect.DeepEqual(res, acceptAct) {
		t.Errorf("expected %#v, got %#v", acceptAct, res)
	}
	switch {
	case len(mods) != 1:
		t.Errorf("expected 1 modification, got %v", len(mods))
	case mods[0].Code != milter.ActInsertHeader:
		t.Errorf("expected modification code %q, got %q", milter.ActInsertHeader, mods[0].Code)
	case mods[0].HeaderName != "Authentication-Results":
		t.Errorf("expected insert header name %q, got %q", "Authentication-Results", mods[0].HeaderName)
	case mods[0].HeaderValue != expectedField:
		t.Errorf("expected insert header value %q, got %q", expectedField, mods[0].HeaderValue)
	case mods[0].HeaderIndex != 0:
		t.Errorf("expected insert header index %q, got %q", 0, mods[0].HeaderIndex)
	default:
		goto skiplog
	}
	t.Log(mods)

skiplog:
	for _, expected := range expectedOut {
		if !bytes.Contains(out.Bytes(), []byte(expected)) {
			t.Errorf("expected contains:\n%s\nactual:\n%s", expected, out.String())
		}
	}

	// Assert all log lines are prefixed with the queue ID.
	expectedPrefix := "QUEUEID:"
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		line := string(scanner.Bytes())
		if !strings.HasPrefix(line, expectedPrefix) {
			t.Errorf("expected log lines to be prefixed with: %q\nactual:\n%s", expectedPrefix, line)
		}
	}
}

func TestUnAuthenticatedClient(t *testing.T) {
	config := `ListenURI = "tcp://127.0.0.1:"`
	network, address, out := setup(t, config)

	client := milter.NewClientWithOptions(network, address, milter.ClientOptions{
		Dialer: &net.Dialer{},
	})
	defer client.Close()
	session, err := client.Session()
	if err != nil {
		t.Fatal("unexpected error: ", err)
	}
	defer session.Close()

	res, err := session.Mail("nicolas@example.fr", []string{})
	if err != nil {
		t.Error("unexpected err sending MAIL FROM: ", err)
	}
	expectedAct := &milter.Action{Code: milter.ActAccept}
	if !reflect.DeepEqual(expectedAct, res) {
		t.Errorf("expected %#v, got %#v", expectedAct, res)
	}

	if out.Len() != 0 {
		t.Errorf("expected empty log output, got %q", out.String())
	}
}
