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
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/emersion/go-milter"
	"github.com/emersion/go-msgauth/authres"
)

type Conf struct {
	AuthservID string
	ListenURI  string
	UMask      int
}

// Default values
var conf = Conf{
	ListenURI: "unix:///run/homard/homard.sock",
	UMask:     0o002,
}

// Set by the compiler
var version = "unknown"

var rejectDomains = make(map[string]bool)

var l *log.Logger = log.New(os.Stderr, "", 0)

type Session struct {
	milter.NoOpMilter
}

func (s *Session) MailFrom(from string, m *milter.Modifier) (milter.Response, error) {
	// Only process mails from authenticated clients, e.g. SASL authenticated in Postfix.
	if m.Macros["{auth_authen}"] != "" {
		return milter.RespContinue, nil
	}
	return milter.RespAccept, nil
}

func (s *Session) Body(m *milter.Modifier) (milter.Response, error) {
	queueID := m.Macros["i"]
	login := m.Macros["{auth_authen}"]
	results := []authres.Result{
		&authres.AuthResult{Value: authres.ResultPass, Auth: login},
	}
	m.InsertHeader(0, "Authentication-Results", authres.Format(conf.AuthservID, results))
	l.Printf("%s: Authentication-Results field added", queueID)
	return milter.RespAccept, nil
}

const (
	usageFmt = `Usage: homard [OPTION]...

Milter to add SMTP AUTH Authentication-Results field to self-sent mails.

Options:
  -c FILE       Read config from FILE. (default %q)
  -h, --help    Show this help and exit.
  --version     Show version and exit.
`
	flagConfDef = "/etc/homard.conf"
)

func main() {
	cli := flag.NewFlagSet("homard", flag.ExitOnError)
	cli.Usage = func() {
		fmt.Fprintf(cli.Output(), usageFmt, flagConfDef)
	}
	var (
		flagConf    string
		flagHelp    bool
		flagVersion bool
	)
	cli.StringVar(&flagConf, "c", flagConfDef, "")
	cli.BoolVar(&flagHelp, "h", false, "")
	cli.BoolVar(&flagHelp, "help", false, "")
	cli.BoolVar(&flagVersion, "version", false, "")
	cli.Parse(os.Args[1:])

	if flagHelp {
		cli.SetOutput(os.Stdout)
		cli.Usage()
		os.Exit(0)
	}

	if flagVersion {
		fmt.Println("homard", version)
		os.Exit(0)
	}

	conffile, err := os.Open(flagConf)
	if err != nil {
		l.Fatal("Failed to open conf file: ", err)
	}
	decoder := toml.NewDecoder(conffile)
	if _, err := decoder.Decode(&conf); err != nil {
		l.Fatalf("Failed to parse conf file %s: %v", flagConf, err)
	}

	if conf.AuthservID == "" {
		var err error
		conf.AuthservID, err = os.Hostname()
		if err != nil {
			l.Fatal("Failed to read hostname: ", err)
		}
	}

	network, address, found := strings.Cut(conf.ListenURI, "://")
	if !found {
		l.Fatal("Invalid listen URI")
	}

	s := milter.Server{
		NewMilter: func() milter.Milter {
			return &Session{}
		},
		Protocol: milter.OptNoConnect | milter.OptNoHelo | milter.OptNoRcptTo | milter.OptNoHeaders | milter.OptNoBody,
	}

	// Allows to set the permissions of the created unix socket
	syscall.Umask(conf.UMask)

	ln, err := net.Listen(network, address)
	if err != nil {
		l.Fatal("Failed to setup listener: ", err)
	}

	// Closing the listener will unlink the unix socket, if any
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		if err := s.Close(); err != nil {
			l.Fatal("Failed to close server: ", err)
		}
	}()

	l.Printf("Milter listening at %s://%v", ln.Addr().Network(), ln.Addr())
	if err := s.Serve(ln); err != nil && err != milter.ErrServerClosed {
		l.Fatal("Failed to serve: ", err)
	}
}
