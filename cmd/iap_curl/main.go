package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/b4b4r07/iap_curl"
)

const (
	app     = "iap_curl"
	version = "0.1.1"
)

const help = `iap_curl - curl wrapper for making HTTP request to IAP-protected app

Usage:
  iap_curl [flags] URL

Flags:
  --edit, --edit-config   edit config
  --list, --list-urls     list URLs described in config
  --help                  show help message
  --version               show version
`

// CLI represents the attributes for command-line interface
type CLI struct {
	opt  option
	args []string
	urls []url.URL
	cfg  iap.Config

	stdout io.Writer
	stderr io.Writer
}

type option struct {
	version bool
	help    bool
	list    bool
	edit    bool
}

func main() {
	os.Exit(newCLI(os.Args[1:]).run())
}

func newCLI(args []string) CLI {
	var c CLI

	c.stdout = os.Stdout
	c.stderr = os.Stderr

	// Do not handle error
	c.cfg.Load()

	for _, arg := range args {
		switch arg {
		case "--help":
			c.opt.help = true
		case "--version":
			c.opt.version = true
		case "--list", "--list-urls":
			c.opt.list = true
		case "--edit", "--edit-config":
			c.opt.edit = true
		default:
			u, err := url.ParseRequestURI(arg)
			if err == nil {
				c.urls = append(c.urls, *u)
			} else {
				c.args = append(c.args, arg)
			}
		}
	}

	return c
}

func (c CLI) exit(msg interface{}) int {
	switch m := msg.(type) {
	case int:
		return m
	case nil:
		return 0
	case string:
		fmt.Fprintf(c.stdout, "%s\n", m)
		return 0
	case error:
		fmt.Fprintf(c.stderr, "[ERROR] %s: %s\n", app, m.Error())
		return 1
	default:
		panic(msg)
	}
}

func (c CLI) run() int {
	if c.opt.help {
		return c.exit(help)
	}

	if c.opt.version {
		return c.exit(fmt.Sprintf("%s v%s (runtime: %s)", app, version, runtime.Version()))
	}

	if c.opt.list {
		return c.exit(strings.Join(c.cfg.GetURLs(), "\n"))
	}

	if c.opt.edit {
		return c.exit(c.cfg.Edit())
	}

	url := c.getURL()
	if url == "" {
		return c.exit(errors.New("invalid url or url not given"))
	}

	env, err := c.cfg.GetEnv(url)
	if err != nil {
		return c.exit(err)
	}

	app, err := iap.New(env.Credentials, env.ClientID)
	if err != nil {
		return c.exit(err)
	}
	token, err := app.GetToken()
	if err != nil {
		return c.exit(err)
	}

	if !c.cfg.Registered(url) {
		c.cfg.Register(iap.Service{
			URL: url,
			Env: env,
		})
	}

	authHeader := fmt.Sprintf("'Authorization: Bearer %s'", token)
	args := append(
		[]string{"-H", authHeader}, // For IAP
		c.args...,
	)
	args = append(args, url)

	s := newShell(env.Binary, args)
	return c.exit(s.run())
}

func (c CLI) debug(a ...interface{}) {
	fmt.Fprint(c.stderr, a...)
}

func (c CLI) getURL() string {
	if len(c.urls) == 0 {
		return ""
	}
	return c.urls[0].String()
}

type shell struct {
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	env     map[string]string
	command string
	args    []string
}

func newShell(command string, args []string) shell {
	return shell{
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
		env:     map[string]string{},
		command: command,
		args:    args,
	}
}

func (s shell) run() error {
	command := s.command
	if _, err := exec.LookPath(command); err != nil {
		return err
	}
	for _, arg := range s.args {
		command += " " + arg
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Stderr = s.stderr
	cmd.Stdout = s.stdout
	cmd.Stdin = s.stdin
	for k, v := range s.env {
		cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s", k, v))
	}
	return cmd.Run()
}
