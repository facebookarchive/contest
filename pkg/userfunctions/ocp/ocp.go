// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package ocp

import (
	"encoding/json"
	"io/ioutil"
)

var parameterFile = "parameters.json"

type fqdnParameter struct {
	PrivateKeyFile string
	BmcHost        string
	BmcUser        string
	BmcPassword    string
	SlotToTest     string
	GithubRepo     string
}

type contestParameter struct {
	ID        string
	Parameter fqdnParameter
}

var userFunctions = map[string]interface{}{
	"getPrivateKeyFile": func(a string) (string, error) {
		fb, err := ioutil.ReadFile(parameterFile)
		if err != nil {
			return "", err
		}
		var parameter []contestParameter
		err = json.Unmarshal(fb, &parameter)
		if err != nil {
			return "", err
		}
		for _, value := range parameter {
			if value.ID == a {
				return value.Parameter.PrivateKeyFile, nil
			}
		}

		return "", nil
	},
	"getBmcHost": func(a string) (string, error) {
		fb, err := ioutil.ReadFile(parameterFile)
		if err != nil {
			return "", err
		}
		var parameter []contestParameter
		err = json.Unmarshal(fb, &parameter)
		if err != nil {
			return "", err
		}
		for _, value := range parameter {
			if value.ID == a {
				return value.Parameter.BmcHost, nil
			}
		}
		return "", nil
	},
	"getBmcUser": func(a string) (string, error) {
		fb, err := ioutil.ReadFile(parameterFile)
		if err != nil {
			return "", err
		}
		var parameter []contestParameter
		err = json.Unmarshal(fb, &parameter)
		if err != nil {
			return "", err
		}
		for _, value := range parameter {
			if value.ID == a {
				return value.Parameter.BmcUser, nil
			}
		}
		return "", nil
	},
	"getBmcPassword": func(a string) (string, error) {
		fb, err := ioutil.ReadFile(parameterFile)
		if err != nil {
			return "", err
		}
		var parameter []contestParameter
		err = json.Unmarshal(fb, &parameter)
		if err != nil {
			return "", err
		}
		for _, value := range parameter {
			if value.ID == a {
				return value.Parameter.BmcPassword, nil
			}
		}
		return "", nil
	},
	"getSlotToTest": func(a string) (string, error) {
		fb, err := ioutil.ReadFile(parameterFile)
		if err != nil {
			return "", err
		}
		var parameter []contestParameter
		err = json.Unmarshal(fb, &parameter)
		if err != nil {
			return "", err
		}
		for _, value := range parameter {
			if value.ID == a {
				return value.Parameter.SlotToTest, nil
			}
		}
		return "", nil
	},
	"getGithubRepo": func(a string) (string, error) {
		fb, err := ioutil.ReadFile(parameterFile)
		if err != nil {
			return "", err
		}
		var parameter []contestParameter
		err = json.Unmarshal(fb, &parameter)
		if err != nil {
			return "", err
		}
		for _, value := range parameter {
			if value.ID == a {
				return value.Parameter.GithubRepo, nil
			}
		}
		return "", nil
	},
	"getTty": func(a string) (string, error) {
		fb, err := ioutil.ReadFile(parameterFile)
		if err != nil {
			return "", err
		}
		var parameter []contestParameter
		err = json.Unmarshal(fb, &parameter)
		if err != nil {
			return "", err
		}
		for _, value := range parameter {
			if value.ID == a {
				switch value.Parameter.SlotToTest {
				case "slot1":
					return "/dev/ttyS1", nil
				case "slot2":
					return "/dev/ttyS2", nil
				case "slot3":
					return "/dev/ttyS3", nil
				case "slot4":
					return "/dev/ttyS4", nil
				}
			}
		}
		return "", nil
	},
}

// Load - Return the user-defined functions
func Load() map[string]interface{} {
	return userFunctions
}
