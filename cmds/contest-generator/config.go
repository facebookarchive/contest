package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v2"
)

// I don't know what a valid charset for alias imports is, so here I am
// allowing case-insensitive alphanumeric strings that start with a letter
// and can contain underscores.
var aliasRegexp = regexp.MustCompile("^(?i)[a-z][0-9a-z_]+$")

// Config is a configuration object mapped to the plugin configuration file.
type Config struct {
	ConfigFile     string
	TargetManagers ConfigEntries
	TestFetchers   ConfigEntries
	TestSteps      ConfigEntries
	Reporters      ConfigEntries
}

// ConfigEntry maps a plugin config entry as a couple of import path, and import
// alias
type ConfigEntry struct {
	Path  string
	Alias string
}

// Validate validates the ConfigEntry object.
func (ce ConfigEntry) Validate() error {
	if ce.Path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	// validate that if Alias is present, it's a valid alias string.
	if ce.Alias != "" && !aliasRegexp.MatchString(ce.Alias) {
		return fmt.Errorf("invalid alias '%s', not matching regexp '%s'", ce.Alias, aliasRegexp)
	}
	return nil
}

// ToAlias returns the import alias for the plugin. For example, if the import
// path is github.com/facebookincubator/plugins/teststeps/cmd , it will return
// the string "cmd". If an explicit alias is specified in the `Alias` attribute,
// that string will be returned instead.
func (ce ConfigEntry) ToAlias() string {
	if ce.Alias != "" {
		return ce.Alias
	}
	return path.Base(ce.Path)
}

func (ce ConfigEntry) String() string {
	ret := ce.Path
	if ce.Alias != "" {
		ret += " => " + ce.Alias
	}
	return ret
}

// ConfigEntries is a list of ConfigEntry objects.
type ConfigEntries []ConfigEntry

// Validate validates the ConfigEntries object.
func (ces *ConfigEntries) Validate() error {
	if len(*ces) < 1 {
		return fmt.Errorf("no config entry found")
	}
	for _, ce := range *ces {
		if err := ce.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) String() string {
	pp := func(ee ConfigEntries) string {
		var ret string
		for _, e := range ee {
			ret += "    " + e.String() + "\n"
		}
		return ret
	}
	ret := "TargetManagers\n" + pp(c.TargetManagers)
	ret += "TestFetchers\n" + pp(c.TestFetchers)
	ret += "TestSteps\n" + pp(c.TestSteps)
	ret += "Reporters\n" + pp(c.Reporters)
	return ret
}

// Validate validates the Config object.
func (c *Config) Validate() error {
	if err := c.TargetManagers.Validate(); err != nil {
		return fmt.Errorf("target managers validation failed: %w", err)
	}
	if err := c.TestFetchers.Validate(); err != nil {
		return fmt.Errorf("test fetchers validation failed: %w", err)
	}
	if err := c.TestSteps.Validate(); err != nil {
		return fmt.Errorf("test steps validation failed: %w", err)
	}
	if err := c.Reporters.Validate(); err != nil {
		return fmt.Errorf("reporters validation failed: %w", err)
	}
	// ensure that there are no duplicate package names
	// map of alias => plugintypes
	plugins := make(map[string][]string)
	for _, e := range c.TargetManagers {
		plugins[e.ToAlias()] = append(plugins[e.ToAlias()], "targetmanagers")
	}
	for _, e := range c.TestFetchers {
		plugins[e.ToAlias()] = append(plugins[e.ToAlias()], "testfetchers")
	}
	for _, e := range c.TestSteps {
		plugins[e.ToAlias()] = append(plugins[e.ToAlias()], "teststeps")
	}
	for _, e := range c.Reporters {
		plugins[e.ToAlias()] = append(plugins[e.ToAlias()], "reporters")
	}
	var duplicates []string
	for name, ptypes := range plugins {
		if len(ptypes) > 1 {
			duplicates = append(duplicates, name)
		}
	}
	if len(duplicates) > 0 {
		return fmt.Errorf("found %d duplicate plugin(s): %v", len(duplicates), duplicates)
	}
	return nil
}

func readConfig(filename string) (*Config, error) {
	r, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s': %v", filename, err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			log.Printf("Error closing file '%s': %v", filename, err)
		}
	}()
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return parseConfig(filename, data)
}

func parseConfig(filename string, data []byte) (*Config, error) {
	var cfg Config
	cfg.ConfigFile = filename
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}
	// sort package entries
	sort.Slice(cfg.TargetManagers, func(i, j int) bool {
		return strings.Compare(cfg.TargetManagers[i].Path, cfg.TargetManagers[j].Path) < 0
	})
	sort.Slice(cfg.TestFetchers, func(i, j int) bool {
		return strings.Compare(cfg.TestFetchers[i].Path, cfg.TestFetchers[j].Path) < 0
	})
	sort.Slice(cfg.TestSteps, func(i, j int) bool {
		return strings.Compare(cfg.TestSteps[i].Path, cfg.TestSteps[j].Path) < 0
	})
	sort.Slice(cfg.Reporters, func(i, j int) bool {
		return strings.Compare(cfg.Reporters[i].Path, cfg.Reporters[j].Path) < 0
	})
	// validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	return &cfg, nil
}
