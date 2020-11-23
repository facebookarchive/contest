// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package ocp

import (
	"encoding/json"
	"errors"
	"io/ioutil"
)

var name = "OCP"
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
	"getPrivateKeyFile": func(a ...string) (string, error) {
		if len(a) == 0 {
			return "", errors.New("hostparam: no arg specified")
		}
		if len(a) > 1 {
			return "", errors.New("hostparam: too many parameters")
		}
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
			if value.ID == a[0] {
				return value.Parameter.PrivateKeyFile, nil
			}
		}

		return "", nil
	},
	"getBmcHost": func(a ...string) (string, error) {
		if len(a) == 0 {
			return "", errors.New("hostparam: no arg specified")
		}
		if len(a) > 1 {
			return "", errors.New("hostparam: too many parameters")
		}
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
			if value.ID == a[0] {
				return value.Parameter.BmcHost, nil
			}
		}
		return "", nil
	},
	"getBmcUser": func(a ...string) (string, error) {
		if len(a) == 0 {
			return "", errors.New("hostparam: no arg specified")
		}
		if len(a) > 1 {
			return "", errors.New("hostparam: too many parameters")
		}
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
			if value.ID == a[0] {
				return value.Parameter.BmcUser, nil
			}
		}
		return "", nil
	},
	"getBmcPassword": func(a ...string) (string, error) {
		if len(a) == 0 {
			return "", errors.New("hostparam: no arg specified")
		}
		if len(a) > 1 {
			return "", errors.New("hostparam: too many parameters")
		}
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
			if value.ID == a[0] {
				return value.Parameter.BmcPassword, nil
			}
		}
		return "", nil
	},
	"getSlotToTest": func(a ...string) (string, error) {
		if len(a) == 0 {
			return "", errors.New("hostparam: no arg specified")
		}
		if len(a) > 1 {
			return "", errors.New("hostparam: too many parameters")
		}
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
			if value.ID == a[0] {
				return value.Parameter.SlotToTest, nil
			}
		}
		return "", nil
	},
	"getGithubRepo": func(a ...string) (string, error) {
		if len(a) == 0 {
			return "", errors.New("hostparam: no arg specified")
		}
		if len(a) > 1 {
			return "", errors.New("hostparam: too many parameters")
		}
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
			if value.ID == a[0] {
				return value.Parameter.GithubRepo, nil
			}
		}
		return "", nil
	},
	"getTty": func(a ...string) (string, error) {
		if len(a) == 0 {
			return "", errors.New("hostparam: no arg specified")
		}
		if len(a) > 1 {
			return "", errors.New("hostparam: too many parameters")
		}
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
			if value.ID == a[0] {
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

// Name - Return the Name of the UDFs
func Name() string {
	return name
}

// Load - Return the user-defined functions
func Load() map[string]interface{} {
	return userFunctions
}
