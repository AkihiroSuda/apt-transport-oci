package apt

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type Config map[string]string

// ParseConfig reads the Config-Item fields of msg, as pkgAcqMethod::Configuration.
func ParseConfig(msg *Message) Config {
	c := make(Config)
	for _, item := range msg.Fields.Values("Config-Item") {
		key, value, _ := strings.Cut(item, "=")
		c[strings.ToLower(unquote(key))] = unquote(value)
	}
	return c
}

// unquote undoes apt's QuoteString, keeping s if it has a malformed escape.
func unquote(s string) string {
	if u, err := url.PathUnescape(s); err == nil {
		return u
	}
	return s
}

// Find returns key, or def if it is unset or empty, as Configuration::Find.
func (c Config) Find(key, def string) string {
	if v := c[strings.ToLower(key)]; v != "" {
		return v
	}
	return def
}

// FindI returns the leading integer of key, or def, as Configuration::FindI.
func (c Config) FindI(key string, def int) int {
	var n int
	if _, err := fmt.Sscan(c.Find(key, ""), &n); err != nil {
		return def
	}
	return n
}

// FindB returns key as a boolean, or def, as Configuration::FindB.
func (c Config) FindB(key string, def bool) bool {
	v := c.Find(key, "")
	if n, err := strconv.ParseInt(v, 0, 0); err == nil && (n == 0 || n == 1) {
		return n == 1
	}
	switch strings.ToLower(v) {
	case "no", "false", "without", "off", "disable":
		return false
	case "yes", "true", "with", "on", "enable":
		return true
	}
	return def
}
