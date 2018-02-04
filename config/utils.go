package config

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
)

// getConfigPath populates the configPath global variable, returning errors in the process,
// and, if any, leaves configPath unchanged.
func getConfigPath() error {
	configPathSuffix := "/drivesync/config.json"
	usr, err := user.Current()
	if err != nil {
		return err
	}
	configRoots := []string{
		"",
		"/etc",
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		configRoots[0] = v
	} else {
		configRoots[0] = usr.HomeDir + "/.config"
	}
	var found bool
	for _, v := range configRoots {
		path := filepath.Clean(v) + configPathSuffix
		fi, err := os.Stat(path)
		if err == nil && !fi.IsDir() {
			configPath = path
			found = true
			break
		}
	}
	if !found {
		// we'll need to create the config file
		if usr.Uid == "0" {
			configPath = configRoots[1] + configPathSuffix
		} else {
			configPath = configRoots[0] + configPathSuffix
		}
	}
	return nil
}

// ReadConfig finds, processes and loads the configuration file, and sets the global Config item
// to the struct produced. It returns any errors that happen in the process.
func ReadConfig(isDaemon bool) error {
	err := getConfigPath()
	if err != nil {
		return errors.New(fmt.Sprintf("failed to get config path: %v", err))
	}
	usr, err := user.Current()
	if err != nil {
		return err
	}
	pathUser := usr.HomeDir + "/drivesyncd"
	if _, err := os.Stat(configPath); err != nil {
		log.Print("W: Config file doesn't exist. Creating a default one...")
		parentPath, _ := filepath.Split(configPath)
		err := os.MkdirAll(parentPath, 0755)
		if err != nil {
			return errors.New(fmt.Sprintf("failed to get parent of config path %q: %v", configPath, err))
		}
		f, err := os.Create(configPath)
		if err != nil {
			return errors.New(fmt.Sprintf("failed to open config file: %v", err))
		}
		defer f.Close()
		var logPath, pidPath string
		if usr.Uid == "0" {
			logPath = "/var/log"
			pidPath = "/var/run"
		} else {
			logPath = pathUser
			pidPath = pathUser
		}
		newConfig := config{
			ArchiveRootName:   ArchiveRootName,
			ClientSecretPath:  parentPath + "client_secret.json",
			CreateMissing:     CreateMissing,
			DefaultCategory:   Category,
			ForceRecheck:      ForceRecheck,
			LogFile:           logPath + "/drivesyncd.log",
			PidFile:           pidPath + "/drivesyncd.pid",
			RetryRatio:        RetryRatio,
			RetryStartingRate: RetryStartingRate,
			ScanInterval:      ScanInterval,
			Verbose:           Verbose,
			UseProxy:          UseProxy,
		}
		Config.Set(newConfig)
		b, err := json.MarshalIndent(newConfig, "", "\t")
		if err != nil {
			return errors.New(fmt.Sprintf("failed to marshal config: %v", err))
		}
		_, err = f.Write(b)
		if err != nil {
			return errors.New(fmt.Sprintf("failed to write the default config file: %v", err))
		}
		log.Printf("I: Default config file created at %q.", configPath)
		if isDaemon {
			return errors.New(`please set field "target" in the configuration file`)
		} else {
			return nil
		}
	}
	f, err := os.Open(configPath)
	if err != nil {
		return errors.New(fmt.Sprintf("failed to read config file: %v", err))
	}
	dec := json.NewDecoder(f)
	var newConfig config
	if err := dec.Decode(&newConfig); err != nil {
		return errors.New(fmt.Sprintf("failed to decode config file: %v", err))
	}
	if newConfig.Target != "" {
		newConfig.Target = filepath.Clean(newConfig.Target)
	} else if isDaemon {
		return errors.New(`please set field "target" in the configuration file`)
	}
	if _, err := time.ParseDuration(newConfig.ScanInterval); err != nil {
		return errors.New(fmt.Sprintf("failed to parse scan-interval: %v", err))
	}
	if usr.Uid != "0" {
		f, err := os.Create(filepath.Dir(newConfig.LogFile) + "/.test-drivesyncd")
		if err != nil {
			log.Printf("W: Failed to open log file for writing: %v", err)
			log.Print("W: Falling back to storing things under user home...")
			err = os.MkdirAll(pathUser, os.ModePerm)
			if err != nil {
				log.Fatalf("E: %v", err)
			}
			newConfig.LogFile = pathUser + "/drivesyncd.log"
			newConfig.PidFile = pathUser + "/drivesyncd.pid"
		} else {
			f.Close()
			os.Remove(f.Name())
		}
	}
	// check the proxy settings
	var proxyUrl *url.URL
	if newConfig.UseProxy {
		if newConfig.ProxyURL != "" {
			proxyUrl, err = url.Parse(newConfig.ProxyURL)
			if err != nil {
				return err
			}
		} else {
			return errors.New(`set up to use proxy yet "proxy-url" not set in configuration`)
		}
	}
	// checks finished, apply the new configuration
	Config.Set(newConfig)
	if newConfig.UseProxy {
		http.DefaultTransport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
	}
	return nil
}

func ReloadConfig() error {
	log.Print("I: Reloading configuration file...")
	err := ReadConfig(true)
	if err != nil {
		return err
	}
	return nil
}
