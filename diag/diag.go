// Copyright (c) 2018
// Author: Jeff Weisberg <jaw @ tcp4me.com>
// Created: 2018-Ju8-24 14:01 (EDT)
// Function: AC style diagnostics+logging

/*
in config file:
    debug section

at top of file:
    var dl = diag.Logger("section")

in code:
    dl.Debug(...)
    dl.Verbose(...)
    ...
*/

// diagnostics + logging
package diag

import (
	"context"
	"flag"
	"fmt"
	"log/syslog"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"
)

// defaults
const (
	stack_max = 1048576
)

var hostname = "?"
var progname = "?"
var debugall = false
var usestderr = true

var lock sync.RWMutex
var config = &Config{}
var defaultDiag = &Diag{section: "default", uplevel: 3}
var slog *syslog.Writer

type Diag struct {
	section  string
	uplevel  int
	mailto   string
	mailfrom string
}

type Config struct {
	Mailto   string
	Mailfrom string
	Facility string
	Debug    map[string]bool
}

type logconf struct {
	logprio    syslog.Priority
	to_stderr  bool
	to_email   bool
	with_info  bool
	with_trace bool
}

func init() {
	flag.BoolVar(&debugall, "d", false, "enable all debugging")

	hostname, _ = os.Hostname()
	prog, _ := os.Executable()
	progname = path.Base(prog)
}

func (d *Diag) WithMailTo(e string) *Diag {
	var n Diag
	n = *d
	n.mailto = e
	return &n
}

func (d *Diag) WithMailFrom(e string) *Diag {
	var n Diag
	n = *d
	n.mailfrom = e
	return &n
}

func (d *Diag) Verbose(format string, args ...interface{}) {
	diag(logconf{
		logprio:   syslog.LOG_INFO,
		to_stderr: true,
	}, d, format, args)
}

func (d *Diag) Debug(format string, args ...interface{}) {

	var cf = getConfig()

	if !debugall && !cf.Debug[d.section] && !cf.Debug["all"] {
		return
	}

	diag(logconf{
		logprio:   syslog.LOG_DEBUG,
		to_stderr: true,
		with_info: true,
	}, d, format, args)
}

func (d *Diag) Problem(format string, args ...interface{}) {
	diag(logconf{
		logprio:   syslog.LOG_WARNING,
		to_stderr: true,
		to_email:  true,
		with_info: true,
	}, d, format, args)
}

func (d *Diag) Bug(format string, args ...interface{}) {
	diag(logconf{
		logprio:    syslog.LOG_ERR,
		to_stderr:  true,
		to_email:   true,
		with_info:  true,
		with_trace: true,
	}, d, format, args)
}

func (d *Diag) Fatal(format string, args ...interface{}) {
	diag(logconf{
		logprio:    syslog.LOG_ERR,
		to_stderr:  true,
		to_email:   true,
		with_info:  true,
		with_trace: true,
	}, d, format, args)

	os.Exit(-1)
}

// ################################################################

func Verbose(format string, args ...interface{}) {
	defaultDiag.Verbose(format, args...)
}
func Problem(format string, args ...interface{}) {
	defaultDiag.Problem(format, args...)
}
func Bug(format string, args ...interface{}) {
	defaultDiag.Bug(format, args...)
}
func Fatal(format string, args ...interface{}) {
	defaultDiag.Fatal(format, args...)
}

// ################################################################

func Init(prog string) {
	if prog != "" {
		progname = prog
	}
}

func Logger(sect string) *Diag {
	return &Diag{section: sect, uplevel: 2}
}

func (d *Diag) Logger(sect string) *Diag {
	return &Diag{section: sect, uplevel: 2}
}

func SetConfig(cf *Config) {
	lock.Lock()
	defer lock.Unlock()
	config = cf

	if slog == nil {
		openSyslog(cf.Facility)
	}
}

func SetDebugAll(x bool) {
	debugall = x
}
func SetStderr(x bool) {
	usestderr = x
}

func getConfig() *Config {
	lock.RLock()
	defer lock.RUnlock()
	return config
}

// ################################################################

func diag(cf logconf, d *Diag, format string, args []interface{}) {

	var out string

	if cf.with_info {
		pc, file, line, ok := runtime.Caller(d.uplevel)
		if ok {
			// file is full pathname - trim
			fileshort := cleanFilename(file)

			// get function name
			fun := runtime.FuncForPC(pc)
			if fun != nil {
				funName := cleanFunName(fun.Name())
				out = fmt.Sprintf("%s:%d %s(): ", fileshort, line, funName)
			} else {
				out = fmt.Sprintf("%s:%d ?(): ", fileshort, line)
			}
		} else {
			out = "?:?: "
		}
	}

	// remove a trailing newline
	if format[len(format)-1] == '\n' {
		format = format[:len(format)-1]
	}

	out = out + fmt.Sprintf(format, args...)

	if cf.to_stderr && usestderr {
		fmt.Fprintln(os.Stderr, out)
	}

	// syslog
	if slog != nil {
		sendToSyslog(cf.logprio, out)
	}

	// email
	if cf.to_email {
		sendEmail(d, out, cf.with_trace)
	}

}

func sendEmail(d *Diag, txt string, with_trace bool) {

	cf := getConfig()

	if d.mailto == "" {
		d.mailto = cf.Mailto
	}
	if d.mailfrom == "" {
		d.mailfrom = cf.Mailfrom
	}

	if cf == nil || d.mailto == "" || d.mailfrom == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sendmail", "-t", "-f", d.mailfrom)

	p, _ := cmd.StdinPipe()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Start()

	go func() {
		fmt.Fprintf(p, "To: %s\nFrom: %s\nSubject: %s daemon error\n\n",
			d.mailto, d.mailfrom, progname)

		fmt.Fprintf(p, "an error was detected in %s\n\nhost:   %s\npid:    %d\n\n",
			progname, hostname, os.Getpid())

		fmt.Fprintf(p, "error:\n%s\n", txt)

		if with_trace {
			var stack = make([]byte, stack_max)
			stack = stack[:runtime.Stack(stack, true)]
			fmt.Fprintf(p, "\n\n%s\n", stack)
		}

		p.Close()
	}()

	cmd.Wait()
}

func sendToSyslog(prio syslog.Priority, msg string) {

	switch prio {
	case syslog.LOG_DEBUG:
		slog.Debug(msg)
	case syslog.LOG_INFO:
		slog.Info(msg)
	case syslog.LOG_NOTICE:
		slog.Notice(msg)
	case syslog.LOG_WARNING:
		slog.Warning(msg)
	case syslog.LOG_ERR:
		slog.Err(msg)
	case syslog.LOG_ALERT:
		slog.Alert(msg)
	case syslog.LOG_EMERG:
		slog.Emerg(msg)
	case syslog.LOG_CRIT:
		slog.Crit(msg)
	}
}

var prioName = map[string]syslog.Priority{
	"kern":     syslog.LOG_KERN,
	"user":     syslog.LOG_USER,
	"mail":     syslog.LOG_MAIL,
	"daemon":   syslog.LOG_DAEMON,
	"auth":     syslog.LOG_AUTH,
	"syslog":   syslog.LOG_SYSLOG,
	"lpr":      syslog.LOG_LPR,
	"news":     syslog.LOG_NEWS,
	"uucp":     syslog.LOG_UUCP,
	"cron":     syslog.LOG_CRON,
	"authpriv": syslog.LOG_AUTHPRIV,
	"ftp":      syslog.LOG_FTP,
	"local0":   syslog.LOG_LOCAL0,
	"local1":   syslog.LOG_LOCAL1,
	"local2":   syslog.LOG_LOCAL2,
	"local3":   syslog.LOG_LOCAL3,
	"local4":   syslog.LOG_LOCAL4,
	"local5":   syslog.LOG_LOCAL5,
	"local6":   syslog.LOG_LOCAL6,
	"local7":   syslog.LOG_LOCAL7,
}

func openSyslog(fac string) {

	p, ok := prioName[strings.ToLower(fac)]

	if !ok {
		return
	}

	slog, _ = syslog.New(p, progname)
}

// trim full pathname to dir/file.go
func cleanFilename(file string) string {

	si := strings.LastIndex(file, "/")

	if si == -1 {
		return file
	}

	ssi := strings.LastIndex(file[0:si-1], "/")
	if ssi != -1 {
		si = ssi
	}

	return file[si+1:]
}

func cleanFunName(n string) string {

	dot := strings.LastIndexByte(n, '.')
	if dot != -1 {
		n = n[dot+1:]
	}
	return n
}
