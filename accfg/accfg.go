// Copyright (c) 2018
// Author: Jeff Weisberg <jaw @ tcp4me.com>
// Created: 2018-Jul-24 17:07 (EDT)
// Function: AC style config files

package accfg

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

type conf struct {
	file   string
	lineNo int
}

const DEBUG = false

func Read(file string, cf interface{}) error {

	debugf("read %s\n", file)
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("%s", err)
	}

	defer f.Close()
	fb := bufio.NewReader(f)

	c := &conf{
		file:   file,
		lineNo: 1,
	}

	err = c.readConfig(fb, cf, false)
	if err != nil {
		return fmt.Errorf("cannot parse config '%s': %v", file, err)
	}

	debugf("/read %s\n", file)
	return nil
}

func (c *conf) learnConf(cf interface{}) map[string]int {

	var info = make(map[string]int)
	var val = reflect.ValueOf(cf).Elem()

	for i := 0; i < val.NumField(); i++ {
		// use lower cased field name
		name := strings.ToLower(val.Type().Field(i).Name)
		kind := val.Field(i).Kind().String()
		tags := val.Type().Field(i).Tag

		// override default name
		if n, ok := tags.Lookup("name"); ok {
			name = n
		}

		info[name] = i
		debugf("cf> %s \t%s\t%v\n", name, kind, tags)
	}

	return info
}

func (c *conf) checkAndStore(cf interface{}, info map[string]int, k string, v string) error {

	i, ok := info[k]
	if !ok {
		return fmt.Errorf("invalid param '%s'", k)
	}

	var cfe = reflect.ValueOf(cf).Elem()
	var cfv = cfe.Field(i)
	var tags = cfe.Type().Field(i).Tag

	// RSN - validation

	switch cfv.Kind() {
	case reflect.String:
		cfv.SetString(v)

	case reflect.Int, reflect.Int32, reflect.Int64:
		conv, _ := tags.Lookup("convert")
		var ix int64
		var err error

		switch conv {
		case "duration":
			ix, err = parseDuration(v)
			if err != nil {
				return fmt.Errorf("invalid value for '%s' (expected duration)\n", k)
			}
		default:
			ix, err = strconv.ParseInt(v, 0, 32)
			if err != nil {
				return fmt.Errorf("invalid value for '%s' (expected number)\n", k)
			}
		}
		cfv.SetInt(ix)

	case reflect.Float32, reflect.Float64:
		f, _ := strconv.ParseFloat(v, 64)
		cfv.SetFloat(f)

	case reflect.Bool:
		cfv.SetBool(parseBool(v))

	case reflect.Slice:
		switch cfv.Interface().(type) {
		case []string:
			cfv.Set(reflect.Append(cfv, reflect.ValueOf(v)))
		}

	case reflect.Map:
		// set bool in map
		if cfv.IsNil() {
			cfv.Set(reflect.MakeMap(cfv.Type()))
		}
		cfv.SetMapIndex(reflect.ValueOf(v), reflect.ValueOf(true))

	default:
		return fmt.Errorf("field '%s' has unsupported type (%s)", k, cfv.Kind().String())
	}

	return nil
}

func (c *conf) readConfig(f *bufio.Reader, cf interface{}, isBlock bool) error {

	var cfinfo = c.learnConf(cf)

	for {
		key, delim, err := c.readToken(f, true)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if isBlock && key == "}" {
			return nil
		}

		var val string
		if delim == ' ' {
			val, delim, err = c.readToken(f, false)
			if err != nil {
				return err
			}
		}

		switch {
		case val == "{":
			err = c.readBlock(f, key, cf, cfinfo)
			if err != nil {
				return err
			}
		case key == "include":
			err = c.include(val, cf)
			if err != nil {
				return err
			}
		default:
			debugf(">>> %s => %s\n", key, val)

			err = c.checkAndStore(cf, cfinfo, key, val)
			if err != nil {
				return err
			}
		}
	}
}

func (c *conf) include(file string, cf interface{}) error {
	return Read(c.includeFile(file), cf)

}
func (c *conf) includeFile(file string) string {

	if file[0] == '/' {
		return file
	}

	// if file does not contain a leading path
	// make it relative to the main config file

	dir := path.Dir(c.file)
	debugf("inc dir %s, file %s\n", dir, file)

	if dir == "" {
		return file
	}

	return dir + "/" + file

}

func (c *conf) readToken(f *bufio.Reader, orcolon bool) (string, int, error) {
	var buf []byte

	for {
		c, err := f.ReadByte()
		if err != nil {
			return "", -1, err
		}

		switch c {
		case '#':
			// comment until eol
			err = eatLine(f)
			if err != nil {
				return "", -1, err
			}
			if len(buf) != 0 {
				return string(buf), '\n', nil
			}
			continue
		case '\n':
			if len(buf) != 0 {
				return string(buf), '\n', nil
			}
			continue

		case ':':
			// permit colon to delimit first token
			if !orcolon {
				break
			}
			fallthrough

		case ' ', '\t', '\r':
			if len(buf) != 0 {
				return string(buf), ' ', nil
			}
			continue
		}

		buf = append(buf, c)
	}
}

func (c *conf) readBlock(f *bufio.Reader, sect string, cf interface{}, info map[string]int) error {

	i, ok := info[sect]
	if !ok {
		return fmt.Errorf("invalid section '%s'", sect)
	}

	var cfe = reflect.ValueOf(cf).Elem()
	var cft = cfe.Type().Field(i).Type

	// validate type is slice of pointer to struct
	if cft.Kind() != reflect.Slice || cft.Elem().Kind() != reflect.Ptr || cft.Elem().Elem().Kind() != reflect.Struct {
		panic("invalid config type. must be []*struct")
	}

	// create new one
	var typ = cft.Elem().Elem()
	newcf := reflect.New(typ).Interface()

	// init newcf
	// ...

	var cfv = cfe.Field(i)
	cfv.Set(reflect.Append(cfv, reflect.ValueOf(newcf)))

	err := c.readConfig(f, newcf, true)

	return err
}

func eatLine(f *bufio.Reader) error {
	_, _, err := f.ReadLine()
	return err
}

func debugf(txt string, args ...interface{}) {
	if DEBUG {
		fmt.Printf(txt, args...)
	}
}

// time.Duration is great for short durations (microsecs)
// but useless for real-world durations
// NB: days, months, and years are based on "typical" values and not exact
// returns seconds
func parseDuration(v string) (int64, error) {

	var lc = v[len(v)-1]
	var i int64
	var err error

	if lc >= '0' && lc <= '9' {
		i, err = strconv.ParseInt(v, 0, 32)
	} else {
		i, err = strconv.ParseInt(v[0:len(v)-1], 0, 32)

		switch unicode.ToLower(rune(lc)) {
		case 'y':
			i *= 3600 * 24 * 365
		case 'm':
			i *= 3600 * 24 * 28
		case 'd':
			i *= 3600 * 24
		case 'h':
			i *= 3600
		}

	}

	return i, err
}

func parseBool(v string) bool {

	switch strings.ToLower(v) {
	case "yes", "on", "true", "1":
		return true
	}
	return false
}
